"""Rich renderables built from a Store. Pure functions, easy to test."""
from __future__ import annotations

import time

from rich.panel import Panel
from rich.text import Text

from .. import pricing
from ..model import MAIN
from ..store import Store

FAMILY_COLOR = {"opus": "medium_purple1", "sonnet": "dark_orange", "haiku": "spring_green3", "": "grey62"}
DIM_COLOR = {"opus": "#6a5a9a", "sonnet": "#8a5a1a", "haiku": "#2f7a56", "": "grey42"}
EFFORT_LEVELS = ["low", "medium", "high", "xhigh", "max"]
SPINNER = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
BOX_W = 32
GAP = 2

Cell = tuple[str, str]       # (char, rich style)
Row = list[Cell]


def _age(secs: float) -> str:
    return f"{int(secs // 3600)}h" if secs >= 3600 else f"{int(secs // 60)}m"


def short_model(model: str) -> str:
    fam = pricing.family(model)
    if not fam:
        return model
    digits = "".join(c if c.isdigit() else "." for c in model.split(fam)[-1]).strip(".-")
    ver = ".".join(x for x in digits.split(".") if x)[:5]
    return f"{fam.capitalize()} {ver}".strip()


def fmt_tokens(n: int) -> str:
    if n >= 1_000_000:
        return f"{n / 1_000_000:.1f}M"
    return f"{n / 1000:.0f}k" if n >= 10_000 else (f"{n / 1000:.1f}k" if n >= 1000 else str(n))


def _fit(s: str, width: int) -> str:
    return s if len(s) <= width else s[: width - 1] + "…"


def _row(text: str, style: str, width: int, align: str = "center") -> Row:
    text = _fit(text, width)
    pad = width - len(text)
    left = pad // 2 if align == "center" else 0
    return ([(" ", style)] * left + [(c, style) for c in text]
            + [(" ", style)] * (pad - left))


# -- agent boxes --------------------------------------------------------
def node_box(store: Store, node_id: str, frame: int = 0, now: float | None = None) -> list[Row]:
    n = store.nodes[node_id]
    state = store.state(n, now)
    fam = pricing.family(n.model)
    running = state == "running"
    if state == "failed":
        border = "red"
    else:
        border = FAMILY_COLOR[fam] if running else DIM_COLOR[fam]
    text_style = "" if running else "grey58"
    inner = BOX_W - 4

    title = f" {n.kind} "
    top = [("╭", border)] + [("─", border)] * 1 + [(c, f"bold {border}") for c in _fit(title, BOX_W - 5)]
    top += [("─", border)] * (BOX_W - 1 - len(top)) + [("╮", border)]

    model = short_model(n.model) or "model n/a"
    if n.effort:
        model += f" · {n.effort}"
    spin = SPINNER[frame % len(SPINNER)]
    if node_id == MAIN:
        if state == "stale":
            idle = max(0, (store.clock if now is None else now) - (n.last_active or n.started))
            status, st_style = f"◌ idle {_age(idle)}", "grey50"
        else:
            status, st_style = (f"{spin} {n.activity}" if n.activity else "main session"), "bold"
    elif running:
        status, st_style = f"{spin} {n.activity or 'running'}", "bold green"
    elif state == "failed":
        status, st_style = "✗ failed", "bold red"
    elif state == "stale":
        idle = max(0, (store.clock if now is None else now) - (n.last_active or n.started))
        status, st_style = f"◌ no activity {_age(idle)}", "grey50"
    else:
        status, st_style = "✓ done", "grey58"
    stats = f"{n.turns} turns · {fmt_tokens(n.tokens_in + n.tokens_out)} tok · ~${n.cost:.2f}".replace(" tok", "" if n.turns > 99 or n.cost >= 10 else " tok")

    content = [
        (model, f"bold {border}" if running else border),
        (n.desc, "grey62" if running else "grey42"),
        (status, st_style),
        (stats, "grey62" if running else "grey42"),
    ]
    rows = [top]
    for text, style in content:
        rows.append([("│", border), (" ", "")] + _row(text, style or text_style, inner)
                    + [(" ", ""), ("│", border)])
    rows.append([("╰", border)] + [("─", border)] * (BOX_W - 2) + [("╯", border)])
    return rows


# -- tree layout --------------------------------------------------------
class Block:
    """A rendered subtree: rows of cells, its width, and the column of its top anchor."""

    def __init__(self, rows: list[Row], width: int, anchor: int) -> None:
        self.rows, self.width, self.anchor = rows, width, anchor


def _pad(row: Row, width: int) -> Row:
    return row + [(" ", "")] * (width - len(row))


def _hjoin(blocks: list[Block]) -> tuple[list[Row], list[int], int]:
    """Place blocks side by side; return rows, anchor columns, total width."""
    height = max(len(b.rows) for b in blocks)
    rows: list[Row] = [[] for _ in range(height)]
    anchors, x = [], 0
    for i, b in enumerate(blocks):
        for r in range(height):
            line = b.rows[r] if r < len(b.rows) else []
            rows[r] += _pad(line, b.width) + ([(" ", "")] * GAP if i < len(blocks) - 1 else [])
        anchors.append(x + b.anchor)
        x += b.width + GAP
    return rows, anchors, x - GAP


def _bus(width: int, anchors: list[int], parent: int | None, trunk: str | None, style: str) -> Row:
    """Horizontal connector line above a row of children."""
    chars = [" "] * width
    lo, hi = min(anchors + ([parent] if parent is not None else [])), max(anchors)
    if trunk:
        lo = 0
    for x in range(lo, hi + 1):
        chars[x] = "─"
    for a in anchors:
        chars[a] = "┬"
    if trunk:
        chars[0] = trunk
    elif lo == anchors[0]:
        chars[lo] = "┌" if anchors[0] != parent else "├"
    if not trunk:
        chars[hi] = "┐" if hi == anchors[-1] and len(anchors) > 1 else chars[hi]
    if parent is not None and not trunk:
        chars[parent] = ("│" if anchors == [parent] else "┼") if parent in anchors else "┴"
    return [(c, style) for c in chars]


def subtree(store: Store, node_id: str, max_width: int, frame: int, now: float | None = None,
            show_all: bool = True) -> Block:
    box = node_box(store, node_id, frame, now)
    child_ids = store.visible_children(node_id, now, show_all)
    if not child_ids:
        return Block(box, BOX_W, BOX_W // 2)
    kids = [subtree(store, c, max_width, frame, now, show_all) for c in child_ids]
    line = "grey50"

    # Group children into rows that fit the available width.
    groups: list[list[Block]] = [[]]
    used = 0
    for k in kids:
        if groups[-1] and used + GAP + k.width > max_width - 4:
            groups.append([])
            used = 0
        used += (GAP if groups[-1] else 0) + k.width
        groups[-1].append(k)
    wrapped = len(groups) > 1
    indent = 2 if wrapped else 0

    laid = [_hjoin(g) for g in groups]
    width = max(w for _, _, w in laid) + indent
    rows: list[Row] = []
    first_anchors = None
    for gi, (grows, anchors, gw) in enumerate(laid):
        shift = indent + (0 if wrapped else (width - gw) // 2)
        anchors = [a + shift for a in anchors]
        if gi == 0:
            first_anchors = anchors
            parent = (anchors[0] + anchors[-1]) // 2
        trunk = None
        if wrapped:
            trunk = "┌" if gi == 0 else ("└" if gi == len(laid) - 1 else "├")
        rows.append(_bus(width, anchors, parent if gi == 0 else None, trunk, line))
        for r in grows:
            row = [(" ", "")] * shift + r
            if wrapped and gi < len(laid) - 1:
                row[0] = ("│", line)
            rows.append(_pad(row, width))
        if wrapped and gi < len(laid) - 1:
            pass
    parent_col = parent if not wrapped else (first_anchors[0] + first_anchors[-1]) // 2

    # Parent box centred over its children, with a stem down to the bus line.
    pw = max(width, BOX_W)
    off = max(0, min(pw - BOX_W, parent_col - BOX_W // 2))
    head = [([(" ", "")] * off + r) for r in box]
    stem = [(" ", "")] * parent_col + [("│", line)]
    return Block([_pad(r, pw) for r in head + [stem] + rows], pw, off + BOX_W // 2)


def _to_text(rows: list[Row]) -> Text:
    out = Text(no_wrap=True, overflow="crop")
    for i, row in enumerate(rows):
        run, style = "", None
        for ch, st in row:
            if st != style and run:
                out.append(run, style=style or "")
                run = ""
            style = st
            run += ch
        if run:
            out.append(run, style=style or "")
        if i < len(rows) - 1:
            out.append("\n")
    return out


def tree_view(store: Store, max_width: int = 120, frame: int = 0, now: float | None = None,
              show_all: bool = True) -> Text:
    block = subtree(store, MAIN, max(max_width, BOX_W + 4), frame, now, show_all)
    rows = block.rows
    if not show_all and not store.visible_children(MAIN, now, False):
        msg = "no agent running"
        rows = rows + [[(" ", "")] * max(0, block.anchor - len(msg) // 2) + [(c, "grey50") for c in msg]]
    return _to_text(rows)


# -- side panels --------------------------------------------------------
def legend() -> Text:
    t = Text()
    t.append("CLAUDE CODE AGENT TREE", style="bold")
    for color, label in [("medium_purple1", "opus · advisor"), ("dark_orange", "sonnet · main/worker"),
                         ("spring_green3", "haiku · swarm / jev")]:
        t.append("   ■ ", style=color)
        t.append(label, style="grey62")
    return t


def advisor_view(store: Store) -> Panel:
    a = store.advisor
    t = Text()
    t.append(f"{short_model(a.model) or 'Advisor'}\n", style="bold medium_purple1")
    t.append("calls  ", style="grey50")
    t.append(f"{a.calls}\n", style="bold")
    if a.last_advice:
        t.append("\nlast advice\n", style="grey50")
        t.append(a.last_advice, style="grey85")
    return Panel(t, title="advisor", border_style="medium_purple1", padding=(0, 1), expand=True)


def bar(value: float | None, width: int = 10) -> Text:
    if value is None:
        return Text("░" * width, style="grey30")
    n = round(value * width)
    style = "spring_green3" if value >= 0.7 else "dark_orange"
    t = Text("█" * n, style=style)
    t.append("░" * (width - n), style="grey30")
    return t


def jev_view(store: Store) -> Panel:
    j = store.jev
    t = Text()
    t.append("fork layer   ", style="grey50")
    t.append(f"forks {j.forks}\n\n", style="bold spring_green3")
    for name in sorted(j.decisions):
        conf = j.avg_confidence(name)
        label = name.removeprefix("jev_").replace("_", " ")
        t.append(f"{label:<15}", style="grey70")
        t.append_text(bar(conf))
        t.append(f" {conf:.2f}" if conf is not None else "  n/a", style="grey70" if conf else "grey42")
        esc = int(j.decisions[name][2])
        if esc:
            t.append(f" ↑{esc}", style="dark_orange")
        t.append("\n")
    return Panel(t, title="JEV", border_style="spring_green3", padding=(0, 1), expand=True)


def log_view(store: Store, lines: int = 8) -> Text:
    t = Text(no_wrap=True, overflow="ellipsis")
    for ts, msg in store.log[-lines:]:
        stamp = time.strftime("%H:%M:%S", time.localtime(ts)) if ts else "--:--:--"
        t.append(f"{stamp}  ", style="grey50")
        style = "green" if msg.startswith("+") else ("red" if "failed" in msg else "grey70" if msg.startswith("-") else "medium_purple1")
        t.append(msg + "\n", style=style)
    return t


def status_line(store: Store, mode: str, now: float | None = None) -> Text:
    run, total = store.running_subagents(now)
    t_in, t_out, cost = store.totals()
    return Text(f"{mode} · subagents [{run}/{total} running] · advisor [{store.advisor.calls}]"
                f" · jev [{store.jev.forks} forks] · {fmt_tokens(t_in + t_out)} tok · ~${cost:.2f}",
                style="grey62")
