"""Rich renderables for chat messages. Pure functions of (message, store, now)."""
from __future__ import annotations

import time

from rich.console import Group, RenderableType
from rich.markdown import Markdown
from rich.padding import Padding
from rich.panel import Panel
from rich.table import Table
from rich.text import Text

from .. import pricing
from ..model import MAIN, Message
from ..store import Store
from .render import DIM_COLOR, FAMILY_COLOR, SPINNER, fmt_tokens, short_model

USER_COLOR = "#7cb7ff"
FOLD_LINES = 6
NEST_INDENT = 4


def hhmm(ts: float) -> str:
    return time.strftime("%H:%M", time.localtime(ts)) if ts else "--:--"


def _secs(d: float | None) -> str:
    if d is None:
        return ""
    return f"{d:.1f} s" if d < 60 else f"{int(d // 60)} m {int(d % 60):02d} s"


def agent_of(store: Store, msg: Message):
    return store.get(msg.agent_id)


def agent_label(store: Store, msg: Message) -> str:
    node = agent_of(store, msg)
    return "main" if msg.agent_id == MAIN else (node.kind if node else "agent")


def _color(model: str, dim: bool = False) -> str:
    fam = pricing.family(model)
    return (DIM_COLOR if dim else FAMILY_COLOR)[fam]


def fold(text: str, expanded: bool, limit: int = FOLD_LINES) -> tuple[str, int]:
    """(visible text, number of hidden lines)."""
    lines = text.splitlines()
    if expanded or len(lines) <= limit:
        return text, 0
    return "\n".join(lines[:limit]), len(lines) - limit


def _footer(hidden: int) -> Text:
    return Text(f"… +{hidden} lines  (e to expand)", style="grey50")


def _title(label: str, model: str, ts: float, color: str) -> Text:
    t = Text(f" {label}", style=f"bold {color}")
    if model:
        t.append(f" · {short_model(model)}", style="grey62")
    t.append(f" · {hhmm(ts)} ", style="grey50")
    return t


def _nest(store: Store, msg: Message, r: RenderableType) -> RenderableType:
    """Messages produced by a subagent are indented under the delegation."""
    return Padding(r, (0, 0, 0, NEST_INDENT)) if msg.agent_id != MAIN else r


def _target_state(store: Store, msg: Message, now: float | None) -> str:
    node = store.get(msg.target) if msg.target else None
    return store.state(node, now) if node else (msg.status or "done")


def message_signature(msg: Message, store: Store, now: float | None, expanded: bool, frame: int):
    """Changes whenever the rendered box would change (so views re-render only then)."""
    if msg.role == "tool" and msg.status == "running":
        node = agent_of(store, msg)
        live = bool(node) and store.state(node, now) == "running"
        return (msg.rev, expanded, frame if live else -1, live)
    if msg.role in ("delegation", "report"):
        node = store.get(msg.target) if msg.target else None
        return (msg.rev, expanded, _target_state(store, msg, now), node.cost if node else 0,
                frame if _target_state(store, msg, now) == "running" else -1)
    return (msg.rev, expanded)


def message_renderable(msg: Message, store: Store, now: float | None = None,
                       expanded: bool = False, frame: int = 0) -> RenderableType:
    return {
        "user": _user, "assistant": _assistant, "tool": _tool,
        "delegation": _delegation, "report": _report, "system": _system,
    }.get(msg.role, _system)(msg, store, now, expanded, frame)


def _user(msg, store, now, expanded, frame):
    body, hidden = fold(msg.text, expanded)
    parts: list[RenderableType] = [Text(body)]
    if hidden:
        parts.append(_footer(hidden))
    return Panel(Group(*parts), title=_title("you", "", msg.ts, USER_COLOR), title_align="left",
                 border_style=USER_COLOR, padding=(0, 1))


def _assistant(msg, store, now, expanded, frame):
    node = agent_of(store, msg)
    model = msg.model or (node.model if node else "")
    color = _color(model)
    body, hidden = fold(msg.text, expanded)
    parts: list[RenderableType] = [Markdown(body)]
    if hidden:
        parts.append(_footer(hidden))
    panel = Panel(Group(*parts), title=_title(agent_label(store, msg), model, msg.ts, color),
                  title_align="left", border_style=color, padding=(0, 1))
    return _nest(store, msg, panel)


def _tool(msg, store, now, expanded, frame):
    node = agent_of(store, msg)
    live = bool(node) and store.state(node, now) == "running"
    if msg.status == "error":
        icon = Text("✗", style="bold red")
    elif msg.status == "ok":
        icon = Text("✓", style="green")
    elif live:
        icon = Text(SPINNER[frame % len(SPINNER)], style="bold green")
    else:
        icon = Text("…", style="grey50")           # never answered: interrupted or still unknown
    grid = Table.grid(padding=(0, 1), expand=True)
    grid.add_column(width=1, style="grey30")
    grid.add_column(width=1)
    grid.add_column(no_wrap=True, style="bold")
    grid.add_column(ratio=1, no_wrap=True, overflow="ellipsis", style="grey62")
    grid.add_column(justify="right", no_wrap=True, style="grey50")
    grid.add_row("┃", icon, msg.tool, msg.detail, _secs(msg.duration))
    return _nest(store, msg, Padding(grid, (0, 0, 0, 2)))


def _delegation(msg, store, now, expanded, frame):
    node = store.get(msg.target) if msg.target else None
    model = (node.model if node else "") or msg.model
    color = _color(model)
    state = _target_state(store, msg, now)
    body, hidden = fold(msg.text, expanded, limit=3)
    t = Text()
    if msg.desc:
        t.append(msg.desc + "\n", style="grey62")
    t.append(body)
    chip = Text()
    if state == "running":
        chip.append(f"{SPINNER[frame % len(SPINNER)]} running", style="bold green")
    elif state == "failed":
        chip.append("✗ failed", style="bold red")
    elif state == "stale":
        chip.append("◌ no activity", style="grey50")
    else:
        chip.append("✓ done", style="grey62")
    if msg.duration is not None:
        chip.append(f" · {_secs(msg.duration)}", style="grey50")
    parts: list[RenderableType] = [t]
    if hidden:
        parts.append(_footer(hidden))
    parts.append(chip)
    title = _title(f"→ {msg.kind or 'agent'}", model, msg.ts, color)
    return _nest(store, msg, Panel(Group(*parts), title=title, title_align="left",
                                   border_style=color, padding=(0, 1)))


def _report(msg, store, now, expanded, frame):
    node = store.get(msg.target) if msg.target else None
    failed = msg.status == "failed"
    color = "red" if failed else _color(node.model if node else "")
    kind = node.kind if node else "agent"
    body, hidden = fold(msg.text or "(no report)", expanded)
    parts: list[RenderableType] = [Markdown(body)]
    if hidden:
        parts.append(_footer(hidden))
    title = Text(f" ↩ {kind} {'failed' if failed else 'reported'}", style=f"bold {color}")
    if msg.duration is not None:
        title.append(f" · {_secs(msg.duration)}", style="grey62")
    if node and node.cost_parts:
        title.append(f" · ~${node.cost:,.2f}", style="grey62")
    title.append(f" · {hhmm(msg.ts)} ", style="grey50")
    return _nest(store, msg, Panel(Group(*parts), title=title, title_align="left",
                                   border_style=color, padding=(0, 1)))


def _system(msg, store, now, expanded, frame):
    lines = msg.text.splitlines() or [""]
    if expanded:
        body, hidden = fold(msg.text, True)
        return Panel(Text(body, style="grey62"), title=Text(f" ⚙ system · {hhmm(msg.ts)} ", style="grey50"),
                     title_align="left", border_style="grey30", padding=(0, 1))
    first = lines[0][:110] + ("…" if len(lines[0]) > 110 else "")
    t = Text(no_wrap=True, overflow="ellipsis", style="grey50")
    t.append(f"⚙ {first}")
    if len(lines) > 1:
        t.append(f"  · {len(lines)} lines ▸", style="grey42")
    return Padding(t, (0, 1))
