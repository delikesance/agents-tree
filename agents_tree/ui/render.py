"""Rich renderables built from a Store. Pure functions, easy to test."""
from __future__ import annotations

import time

from rich.columns import Columns
from rich.console import Group
from rich.panel import Panel
from rich.text import Text

from .. import pricing
from ..model import MAIN
from ..store import Store

FAMILY_COLOR = {"opus": "medium_purple1", "sonnet": "orange3", "haiku": "spring_green3", "": "grey62"}
STATUS_STYLE = {"running": ("▶ running", "green"), "done": ("✓ done", "grey62"), "failed": ("✗ failed", "red")}
EFFORT_LEVELS = ["low", "medium", "high", "xhigh", "max"]


def short_model(model: str) -> str:
    fam = pricing.family(model)
    if not fam:
        return model
    digits = "".join(c if c.isdigit() else "." for c in model.split(fam)[-1]).strip(".-")
    ver = ".".join(x for x in digits.split(".") if x)[:5]
    return f"{fam.capitalize()} {ver}".strip()


def effort_bar(effort: str) -> Text:
    t = Text("effort ", style="grey50")
    n = EFFORT_LEVELS.index(effort) + 1 if effort in EFFORT_LEVELS else 0
    t.append("█" * n, style="orange3")
    t.append("░" * (len(EFFORT_LEVELS) - n), style="grey30")
    t.append(f" {effort or '-'}")
    return t


def fmt_tokens(n: int) -> str:
    return f"{n/1000:.0f}k" if n >= 10_000 else (f"{n/1000:.1f}k" if n >= 1000 else str(n))


def node_panel(store: Store, node_id: str) -> Panel:
    n = store.nodes[node_id]
    color = FAMILY_COLOR[pricing.family(n.model)]
    label, style = STATUS_STYLE[n.status] if n.id != MAIN else ("main session", "bold")
    body = Text(justify="center")
    body.append(short_model(n.model) or "model n/a", style=color)
    if n.effort:
        body.append(f" · {n.effort}", style="grey62")
    body.append("\n")
    if n.desc:
        body.append(n.desc[:40] + "\n", style="grey50")
    body.append(label, style=style)
    body.append(f"\n{n.turns} turns · {fmt_tokens(n.tokens_in + n.tokens_out)} tok · ${n.cost:.2f}", style="grey50")
    return Panel(body, title=Text(n.kind, style="bold"), border_style=color, padding=(0, 1), width=34)


def tree_view(store: Store, node_id: str = MAIN) -> Group:
    n = store.nodes[node_id]
    parts = [node_panel(store, node_id)]
    if n.children:
        parts.append(Text("│" if len(n.children) == 1 else "┌" + "─" * 10 + "┴" + "─" * 10 + "┐", style="grey50", justify="center"))
        kids = [tree_view(store, c) for c in n.children]
        parts.append(Columns(kids, align="center", expand=True))
    return Group(*parts)


def advisor_view(store: Store) -> Panel:
    a = store.advisor
    t = Text()
    t.append(f"{short_model(a.model) or 'advisor'} · on call\n", style="bold medium_purple1")
    t.append(f"calls  {a.calls}\n", style="grey70")
    t.append("\nlast advice:\n", style="grey50")
    t.append(a.last_advice or "(none yet)", style="white")
    return Panel(t, title="advisor", border_style="medium_purple1", padding=(0, 1))


def bar(value: float | None, width: int = 12) -> Text:
    if value is None:
        return Text("░" * width, style="grey30")
    n = round(value * width)
    style = "spring_green3" if value >= 0.7 else "orange3"
    t = Text("█" * n, style=style)
    t.append("░" * (width - n), style="grey30")
    return t


def jev_view(store: Store) -> Panel:
    j = store.jev
    t = Text()
    t.append(f"JEV · fork layer   forks {j.forks}\n", style="bold spring_green3")
    for name in sorted(j.decisions):
        conf = j.avg_confidence(name)
        t.append(f"{name.removeprefix('jev_'):<14}")
        t.append_text(bar(conf))
        t.append(f" {conf:.2f}" if conf is not None else " n/a", style="grey70")
        esc = int(j.decisions[name][2])
        if esc:
            t.append(f"  ↑{esc} escalated", style="orange3")
        t.append("\n")
    if not j.decisions:
        t.append("(no jev_* calls yet)", style="grey50")
    return Panel(t, border_style="spring_green3", padding=(0, 1))


def log_view(store: Store, lines: int = 8) -> Text:
    t = Text()
    for ts, msg in store.log[-lines:]:
        stamp = time.strftime("%H:%M:%S", time.localtime(ts)) if ts else "--:--:--"
        t.append(f"{stamp}  ", style="grey50")
        t.append(msg + "\n")
    return t


def status_line(store: Store, mode: str) -> Text:
    run, total = store.running_subagents()
    t_in, t_out, cost = store.totals()
    return Text(f"{mode} · subagents [{run}/{total} running] · advisor [{store.advisor.calls}]"
                f" · jev [{store.jev.forks} forks] · {fmt_tokens(t_in + t_out)} tok · ${cost:.2f}",
                style="grey62")
