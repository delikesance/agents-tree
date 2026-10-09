from __future__ import annotations

from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical, VerticalScroll
from textual.widgets import Footer, Static

from ..sources.live import HookTailer, TranscriptTailer
from ..sources.replay import Replay
from ..store import Store
from . import render


class AgentsTreeApp(App):
    TITLE = "claude code agent tree"
    CSS = """
    #header { height: 1; padding: 0 1; }
    #left { width: 40; }
    #treebox { align-horizontal: center; padding: 1 1; overflow-x: auto; }
    #tree { width: auto; }
    #log { height: 10; border: round $primary-darken-2; padding: 0 1; }
    #status { height: 1; padding: 0 1; }
    """
    BINDINGS = [("q", "quit", "Quit"), ("space", "pause", "Pause (replay)"),
                ("plus,equals_sign", "faster", "Faster"), ("minus", "slower", "Slower")]

    def __init__(self, tailers=None, replay: Replay | None = None, refresh_hz: float = 4.0) -> None:
        super().__init__()
        self.store = Store()
        self.tailers = tailers or []
        self.replay = replay
        self.refresh_hz = refresh_hz
        self.frame = 0

    def compose(self) -> ComposeResult:
        yield Static(render.legend(), id="header")
        with Horizontal():
            with Vertical(id="left"):
                yield Static(id="advisor")
                yield Static(id="jev")
            yield VerticalScroll(Static(id="tree"), id="treebox")
        yield Static(id="log")
        yield Static(id="status")
        yield Footer()

    def on_mount(self) -> None:
        self.set_interval(1 / self.refresh_hz, self.tick)
        self.tick()

    def tick(self) -> None:
        if self.replay:
            events = self.replay.tick(1 / self.refresh_hz)
        else:
            events = [e for t in self.tailers for e in t.poll()]
        for ev in events:
            self.store.apply(ev)
        self.refresh_views()

    def refresh_views(self) -> None:
        s = self.store
        self.query_one("#advisor", Static).update(render.advisor_view(s))
        self.query_one("#jev", Static).update(render.jev_view(s))
        width = max(self.size.width - 44, 40)
        self.frame += 1
        self.query_one("#tree", Static).update(render.tree_view(s, width, self.frame))
        self.query_one("#log", Static).update(render.log_view(s))
        if self.replay:
            r = self.replay
            mode = f"replay {r.pos}/{len(r.events)} x{r.speed:g}" + (" [paused]" if r.paused else "")
        else:
            mode = "live"
        self.query_one("#status", Static).update(render.status_line(s, mode))

    def action_pause(self) -> None:
        if self.replay:
            self.replay.paused = not self.replay.paused

    def action_faster(self) -> None:
        if self.replay:
            self.replay.speed = min(self.replay.speed * 2, 64)

    def action_slower(self) -> None:
        if self.replay:
            self.replay.speed = max(self.replay.speed / 2, 0.25)


def make_live(session_jsonl, hooks: bool = True) -> AgentsTreeApp:
    tailers = [TranscriptTailer(session_jsonl)]
    if hooks:
        tailers.append(HookTailer())
    return AgentsTreeApp(tailers=tailers)
