"""Rich renderables built from a Store. Pure functions, easy to test."""
from __future__ import annotations

import time

from rich.panel import Panel
from rich.text import Text

from .. import pricing
from ..model import MAIN
from ..store import Store, hit_rate

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
    price = f"~${n.cost:,.2f}" if n.cost_parts else "$ n/a"
    if n.unpriced_turns and n.cost_parts:
        price += "+"   # some turns could not be priced
    hit = hit_rate(n.fresh, n.cache_write, n.cache_read)
    stats = f"{n.turns} turns · {price}"
    cache = (f"cache {hit:.0%} · out {fmt_tokens(n.tokens_out)}" if hit is not None
             else f"out {fmt_tokens(n.tokens_out)}")

    content = [
        (model, f"bold {border}" if running else border),
        (n.desc, "grey62" if running else "grey42"),
        (status, st_style),
        (stats, "grey62" if running else "grey42"),
        (cache, "grey62" if running else "grey42"),
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


def cost_view(store: Store) -> Panel:
    """Where the money goes: tokens and USD per category, cache hit rate, savings."""
    s = store.summary()
    c = s.cost
    t = Text()
    rows = [("input (uncached)", s.fresh, c.fresh), ("cache write", s.write, c.write),
            ("cache read", s.read, c.read), ("output", s.out, c.out)]
    for label, tokens, usd in rows:
        t.append(f"{label:<17}", style="grey70")
        t.append(f"{fmt_tokens(tokens):>7}", style="grey85")
        t.append(f"{f'~${usd:,.2f}':>10}\n", style="grey62")
    t.append("─" * 34 + "\n", style="grey30")
    t.append(f"{'total':<17}", style="bold")
    t.append(f"{'':>7}{f'~${c.total:,.2f}':>10}\n", style="bold")
    hit = s.hit_rate
    t.append("cache hit  ", style="grey50")
    t.append(f"{hit:.1%}" if hit is not None else "n/a",
             style="bold spring_green3" if hit and hit >= 0.8 else "bold dark_orange")
    if c.nocache:
        t.append(f"\nsaved ~${c.saved:,.2f}", style="spring_green3")
        t.append(f"  (no cache: ~${c.nocache:,.2f})\n", style="grey50")
    if s.unpriced_turns:
        t.append(f"\n{s.unpriced_turns} turns on an unpriced model are excluded", style="dark_orange")
    return Panel(t, title="cost (estimate)", border_style="grey50", padding=(0, 1), expand=True)


def context_view(comp) -> Panel:
    """Where the input tokens come from (estimate), with what compressing tool outputs would save."""
    t = Text()
    if not comp or not comp.turns:
        t.append("computing…", style="grey50")
        return Panel(t, title="where tokens go", border_style="grey50", padding=(0, 1), expand=True)
    colors = {"context before the first message": "grey62", "code and commands written (tool calls)": "dark_orange",
              "tool outputs": "spring_green3", "your messages": "#7cb7ff", "assistant replies": "medium_purple1"}
    for label, volume in comp.categories():
        share = comp.share(volume)
        t.append(f"{share:>5.1%} ", style="bold")
        t.append_text(bar(min(share / 0.5, 1.0), 8))
        t.append(f" ~${comp.usd(volume):,.2f}\n", style="grey50")
        t.append(f"      {label}\n", style=colors.get(label, "grey50"))
        if label == "tool outputs":
            for name, v in sorted(comp.outputs.items(), key=lambda x: -x[1].volume)[:3]:
                t.append(f"        {name:<10}{comp.share(v.volume):>6.1%}\n", style="grey42")
    share, usd = comp.what_if_compress_outputs()
    t.append(f"\nshrinking tool outputs 70% ≈ −{share:.1%}", style="grey62")
    t.append(f"  (~${usd:,.2f})", style="spring_green3")
    return Panel(t, title="where tokens go (estimate)", border_style="grey50", padding=(0, 1), expand=True)


def status_line(store: Store, mode: str, now: float | None = None) -> Text:
    run, total = store.running_subagents(now)
    s = store.summary()
    hit = f" · cache {s.hit_rate:.0%}" if s.hit_rate is not None else ""
    return Text(f"{mode} · subagents [{run}/{total} running] · advisor [{store.advisor.calls}]"
                f" · jev [{store.jev.forks} forks] · ~${s.cost.total:,.2f}{hit}", style="grey62")
