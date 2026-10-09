"""The agent rail: compact cards for agents that are running right now."""
from __future__ import annotations

from rich.console import Group, RenderableType
from rich.padding import Padding
from rich.panel import Panel
from rich.text import Text

from .. import pricing
from ..model import MAIN
from ..store import Store, hit_rate
from .render import DIM_COLOR, FAMILY_COLOR, SPINNER, _age, fmt_tokens, short_model


def _line(text: str, style: str = "") -> Text:
    return Text(text, style=style, no_wrap=True, overflow="ellipsis")


def card(store: Store, node_id: str, now: float | None, frame: int) -> Panel:
    n = store.nodes[node_id]
    state = store.state(n, now)
    fam = pricing.family(n.model)
    running = state == "running"
    border = "red" if state == "failed" else (FAMILY_COLOR[fam] if running else DIM_COLOR[fam])
    head = short_model(n.model) or "model n/a"
    if n.effort:
        head += f" · {n.effort}"
    spin = SPINNER[frame % len(SPINNER)]
    if state == "stale":
        idle = max(0, (store.clock if now is None else now) - (n.last_active or n.started))
        status = ("◌ idle " if node_id == MAIN else "◌ no activity ") + _age(idle)
        st_style = "grey50"
    elif state == "failed":
        status, st_style = "✗ failed", "bold red"
    elif state == "done":
        status, st_style = "✓ done", "grey58"
    else:
        status = f"{spin} {n.activity or ('working' if node_id != MAIN else 'main session')}"
        st_style = "bold green"
    hit = hit_rate(n.fresh, n.cache_write, n.cache_read)
    price = f"~${n.cost:,.2f}" if n.cost_parts else "$ n/a"
    stats = f"{n.turns} turns · {price}" + (f" · {hit:.0%}" if hit is not None else "")
    rows: list[RenderableType] = [_line(head, f"bold {border}" if running else border)]
    if n.desc:
        rows.append(_line(n.desc, "grey62" if running else "grey42"))
    rows += [_line(status, st_style), _line(stats, "grey50" if running else "grey42")]
    return Panel(Group(*rows), title=Text(f" {n.kind} ", style=f"bold {border}"), title_align="left",
                 border_style=border, padding=(0, 1))


def rail_view(store: Store, now: float | None = None, frame: int = 0, show_all: bool = False) -> RenderableType:
    """Cards for main and its agents; only running ones unless show_all."""
    cards: list[RenderableType] = []

    def walk(node_id: str, depth: int) -> None:
        cards.append(Padding(card(store, node_id, now, frame), (0, 0, 0, min(depth, 3) * 2)))
        for c in store.visible_children(node_id, now, show_all):
            walk(c, depth + 1)

    walk(MAIN, 0)
    running, total = store.running_subagents(now)
    head = Text(f"AGENTS · {running} RUNNING", style="bold grey50")
    out: list[RenderableType] = [head, *cards]
    hidden = total - len([1 for n in store.nodes.values() if n.id != MAIN
                          and (show_all or store.state(n, now) == "running")])
    if not show_all and hidden > 0:
        out.append(Text(f"{hidden} finished or idle hidden · h", style="grey42"))
    elif running == 0 and total == 0:
        out.append(Text("no agent running", style="grey42"))
    return Group(*out)
