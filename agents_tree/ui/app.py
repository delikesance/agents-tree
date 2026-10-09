from __future__ import annotations

from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical, VerticalScroll
from textual.widgets import Footer, Static

import time
from pathlib import Path

from ..sources.live import HookTailer, TranscriptTailer, list_sessions
from ..sources.transcript import read_session
from ..sources.replay import Replay
from ..store import Store
from .picker import SessionPicker
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
    BINDINGS = [("q", "quit", "Quit"), ("s", "pick_session", "Session"),
                ("space", "pause", "Pause"), ("plus,equals_sign", "faster", "Faster"),
                ("minus", "slower", "Slower")]

    def __init__(self, tailers=None, replay: Replay | None = None, refresh_hz: float = 4.0,
                 session: Path | None = None, hooks: bool = True, projects_dir=None) -> None:
        super().__init__()
        self.store = Store()
        self.tailers = tailers or []
        self.replay = replay
        self.refresh_hz = refresh_hz
        self.frame = 0
        self.session = Path(session) if session else None
        self.hooks = hooks
        self.projects_dir = projects_dir
        self.replay_mode = replay is not None

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
        if self.session is None and not self.replay_mode:
            self.action_pick_session()

    # -- session switching ----------------------------------------------
    def action_pick_session(self) -> None:
        sessions = list_sessions(self.projects_dir)
        self.push_screen(SessionPicker(sessions, self.session), self.switch_session)

    def switch_session(self, path: Path | None) -> None:
        """Rebuild the tree for another session (live follow, or replay when in replay mode)."""
        if path is None:
            return
        self.session = path
        self.store = Store()
        if self.replay_mode:
            self.replay = Replay(read_session(path), speed=self.replay.speed if self.replay else 4.0)
        else:
            self.tailers = [TranscriptTailer(path)]
            if self.hooks:
                self.tailers.append(HookTailer(session_id=path.stem))
        self.refresh_views()

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
        now = None if self.replay_mode else time.time()   # replays use event time
        # Only show panels for things that actually happened in this session.
        adv, jev = self.query_one("#advisor", Static), self.query_one("#jev", Static)
        adv.display, jev.display = s.advisor.calls > 0, s.jev.forks > 0
        self.query_one("#left").display = adv.display or jev.display
        if adv.display:
            adv.update(render.advisor_view(s))
        if jev.display:
            jev.update(render.jev_view(s))
        width = max(self.size.width - (44 if self.query_one("#left").display else 4), 40)
        self.frame += 1
        self.query_one("#tree", Static).update(render.tree_view(s, width, self.frame, now))
        self.query_one("#log", Static).update(render.log_view(s))
        if self.replay:
            r = self.replay
            mode = f"replay {r.pos}/{len(r.events)} x{r.speed:g}" + (" [paused]" if r.paused else "")
        else:
            mode = "live"
        mode += f" · session {self.session.stem[:8]}" if self.session else " · no session (press s)"
        self.query_one("#status", Static).update(render.status_line(s, mode, now))

    def action_pause(self) -> None:
        if self.replay:
            self.replay.paused = not self.replay.paused

    def action_faster(self) -> None:
        if self.replay:
            self.replay.speed = min(self.replay.speed * 2, 64)

    def action_slower(self) -> None:
        if self.replay:
            self.replay.speed = max(self.replay.speed / 2, 0.25)


def make_live(session_jsonl=None, hooks: bool = True, projects_dir=None) -> AgentsTreeApp:
    """Live app; with no session it opens the picker on start."""
    app = AgentsTreeApp(session=session_jsonl, hooks=hooks, projects_dir=projects_dir)
    if session_jsonl:
        app.tailers = [TranscriptTailer(session_jsonl)]
        if hooks:
            app.tailers.append(HookTailer(session_id=Path(session_jsonl).stem))
    return app
