from __future__ import annotations

import os
import time
from pathlib import Path

from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical, VerticalScroll
from textual.widgets import Footer, Input, Static

from .. import composition
from ..model import MAIN
from ..sender import ClaudeSender, find_transcript
from ..sources.live import HookTailer, TranscriptTailer, list_sessions, scan_session
from ..sources.replay import Replay
from ..sources.transcript import read_session
from ..store import Store
from . import render
from .chat import ChatView
from .picker import SessionPicker
from .rail import rail_view


class MessageInput(Input):
    BINDINGS = [("escape", "leave", "Back to chat")]

    def action_leave(self) -> None:
        self.app.query_one(ChatView).focus()


class AgentsTreeApp(App):
    TITLE = "claude code agent tree"
    CSS = """
    #header { height: 1; padding: 0 1; }
    #left { width: 40; padding: 1 0 0 1; }
    #center { width: 1fr; padding: 0 1; }
    #chat { height: 1fr; }
    #treebox { height: 1fr; align-horizontal: center; padding: 1 1; overflow-x: auto; }
    #tree { width: auto; }
    #log { height: 10; border: round $primary-darken-2; padding: 0 1; }
    #sendstatus { height: 1; padding: 0 1; }
    #input { height: 3; }
    #status { height: 1; padding: 0 1; }
    """
    BINDINGS = [("q", "quit", "Quit"), ("s", "pick_session", "Session"),
                ("i,enter", "focus_input", "Write"), ("f", "cycle_filter", "Filter"),
                ("e", "toggle_expand", "Expand"), ("t", "toggle_tree", "Tree"),
                ("h", "toggle_history", "History"), ("ctrl+k", "interrupt", "Stop Claude"),
                ("space", "pause", "Pause"), ("plus,equals_sign", "faster", "Faster"),
                ("minus", "slower", "Slower")]

    def __init__(self, tailers=None, replay: Replay | None = None, refresh_hz: float = 4.0,
                 session: Path | None = None, hooks: bool = True, projects_dir=None,
                 permissions: str = "all", claude_bin: str = "claude", can_send: bool = True) -> None:
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
        self.show_all = self.replay_mode   # live: only agents running now; replay: everything
        self.view = "chat"                 # "chat" | "tree"
        self.filter: str | None = None     # chat filter: None = all, else an agent node id
        self.permissions, self.claude_bin = permissions, claude_bin
        self.can_send = can_send and not self.replay_mode
        self.sender: ClaudeSender | None = None
        self._new_session_id: str | None = None   # a session we started ourselves, transcript pending
        self.composition: composition.Composition | None = None
        self._comp_for: Path | None = None        # session the composition was computed for
        self._comp_at = 0.0
        self._comp_busy = False

    def compose(self) -> ComposeResult:
        yield Static(render.legend(), id="header")
        with Horizontal():
            with VerticalScroll(id="left"):
                yield Static(id="agents")
                yield Static(id="cost")
                yield Static(id="context")
                yield Static(id="advisor")
                yield Static(id="jev")
            with Vertical(id="center"):
                yield ChatView(id="chat")
                yield VerticalScroll(Static(id="tree"), id="treebox")
                yield Static(id="log")
                yield Static(id="sendstatus")
                yield MessageInput(placeholder="Message Claude…  (enter: send · esc: back to chat)", id="input")
        yield Static(id="status")
        yield Footer()

    def on_mount(self) -> None:
        self.set_interval(1 / self.refresh_hz, self.tick)
        self.query_one(ChatView).focus()
        self.tick()
        if self.session is None and not self.replay_mode:
            self.action_pick_session()

    # -- session switching ----------------------------------------------
    def action_pick_session(self) -> None:
        sessions = list_sessions(self.projects_dir)
        self.push_screen(SessionPicker(sessions, self.session), self.switch_session)

    def switch_session(self, path: Path | None) -> None:
        """Rebuild chat and tree for another session (live follow, or replay in replay mode)."""
        if path is None:
            return
        self._drop_sender()
        self.session = path
        self.composition, self._comp_for, self._comp_at = None, None, 0.0
        self.store = Store()
        self.filter = None
        self.query_one(ChatView).reset()
        if self.replay_mode:
            self.replay = Replay(read_session(path), speed=self.replay.speed if self.replay else 4.0)
        else:
            self.tailers = [TranscriptTailer(path)]
            if self.hooks:
                self.tailers.append(HookTailer(session_id=path.stem))
        self.refresh_views()

    # -- sending ----------------------------------------------------------
    def _ensure_sender(self) -> ClaudeSender:
        if self.sender is None:
            if self.session is not None:
                cwd = scan_session(self.session)[1] or os.getcwd()
                self.sender = ClaudeSender(self.session.stem, cwd, resume=True,
                                           permissions=self.permissions, claude_bin=self.claude_bin)
            else:
                self.sender = ClaudeSender(None, os.getcwd(), resume=False,
                                           permissions=self.permissions, claude_bin=self.claude_bin)
                self._new_session_id = self.sender.session_id
        return self.sender

    def _drop_sender(self) -> None:
        if self.sender is not None:
            sender, self.sender = self.sender, None
            self._new_session_id = None
            self.run_worker(sender.stop(), exclusive=False)

    async def on_input_submitted(self, event: Input.Submitted) -> None:
        if event.input.id != "input":
            return
        text = event.value.strip()
        event.input.value = ""
        if not text or not self.can_send:
            return
        sender = self._ensure_sender()
        try:
            await sender.send(text)
        except (OSError, FileNotFoundError):
            pass                              # shown through sender.last_error in the status line
        self.refresh_views()

    def _adopt_new_session(self) -> None:
        """A session we started: follow its transcript as soon as Claude has written it."""
        if self._new_session_id and self.session is None:
            path = find_transcript(self._new_session_id, self.projects_dir)
            if path:
                self.session = path
                self.tailers = [TranscriptTailer(path)]
                if self.hooks:
                    self.tailers.append(HookTailer(session_id=path.stem))
                self._new_session_id = None

    def action_focus_input(self) -> None:
        if self.can_send and self.view == "chat":
            self.query_one(MessageInput).focus()

    def action_interrupt(self) -> None:
        if self.sender is not None:
            self.run_worker(self.sender.interrupt(), exclusive=False)

    async def action_quit(self) -> None:
        if self.sender is not None:
            await self.sender.stop()
        self.exit()

    # -- views ------------------------------------------------------------
    def action_cycle_filter(self) -> None:
        """Chat filter: everyone -> main -> each agent (in order of appearance) -> everyone."""
        order = [None, MAIN] + [n.id for n in self.store.nodes.values() if n.id != MAIN]
        i = order.index(self.filter) if self.filter in order else 0
        self.filter = order[(i + 1) % len(order)]
        chat = self.query_one(ChatView)
        chat.filter = self.filter
        chat.reset()
        self.refresh_views()

    def action_toggle_expand(self) -> None:
        chat = self.query_one(ChatView)
        chat.expanded = not chat.expanded
        chat.reset()
        self.refresh_views()

    def action_toggle_tree(self) -> None:
        self.view = "tree" if self.view == "chat" else "chat"
        self.refresh_views()

    def action_toggle_history(self) -> None:
        self.show_all = not self.show_all
        self.refresh_views()

    def tick(self) -> None:
        if self.replay:
            events = self.replay.tick(1 / self.refresh_hz)
        else:
            self._adopt_new_session()
            events = [e for t in self.tailers for e in t.poll()]
        for ev in events:
            self.store.apply(ev)
        self.refresh_views()

    def _maybe_refresh_composition(self) -> None:
        """Recompute 'where tokens go' in a thread: at most every 15 s while live, once for a replay."""
        if self.session is None or self._comp_busy or not self.session.exists():
            return
        due = self._comp_for != self.session or (not self.replay_mode and time.time() - self._comp_at > 15)
        if not due:
            return
        self._comp_busy, path = True, self.session

        def work():
            try:
                return path, composition.analyze(path)
            except OSError:
                return path, None

        worker = self.run_worker(work, thread=True, exclusive=False)
        self._comp_worker = worker
        self.set_timer(0.05, self._poll_comp)

    def _poll_comp(self) -> None:
        w = getattr(self, "_comp_worker", None)
        if w is None:
            return
        if w.is_finished:
            self._comp_worker = None
            result = w.result if w.result is not None else None
            self._comp_busy = False
            if result:
                path_, comp = result
                if path_ == self.session and comp is not None:
                    self.composition, self._comp_for, self._comp_at = comp, path_, time.time()
        else:
            self.set_timer(0.2, self._poll_comp)

    def refresh_views(self) -> None:
        self._maybe_refresh_composition()
        s = self.store
        now = None if self.replay_mode else time.time()   # replays use event time
        self.frame += 1
        in_chat = self.view == "chat"
        chat, treebox, log = self.query_one(ChatView), self.query_one("#treebox"), self.query_one("#log")
        chat.display, treebox.display, log.display = in_chat, not in_chat, not in_chat
        # Rail: running agents only (h shows the rest), plus panels for things that really happened.
        self.query_one("#agents", Static).update(rail_view(s, now, self.frame, self.show_all))
        adv, jev, cost = (self.query_one(f"#{n}", Static) for n in ("advisor", "jev", "cost"))
        adv.display, jev.display, cost.display = s.advisor.calls > 0, s.jev.forks > 0, s.summary().turns > 0
        ctx = self.query_one("#context", Static)
        ctx.display = cost.display and self.session is not None
        if cost.display:
            cost.update(render.cost_view(s))
        if ctx.display:
            ctx.update(render.context_view(self.composition))
        if adv.display:
            adv.update(render.advisor_view(s))
        if jev.display:
            jev.update(render.jev_view(s))
        if in_chat:
            chat.sync(s, now, self.frame)
        else:
            width = max(self.size.width - 48, 40)
            self.query_one("#tree", Static).update(render.tree_view(s, width, self.frame, now, self.show_all))
            log.update(render.log_view(s))
        self._refresh_input(in_chat)
        if self.replay:
            r = self.replay
            mode = f"replay {r.pos}/{len(r.events)} x{r.speed:g}" + (" [paused]" if r.paused else "")
        else:
            mode = "live"
        mode += f" · {self.view}" + (f" · filter: {self._filter_label()}" if self.filter else "")
        mode += f" · view: {'all' if self.show_all else 'active'}"
        mode += f" · session {self.session.stem[:8]}" if self.session else (
            " · new session" if self._new_session_id else " · no session (press s)")
        self.query_one("#status", Static).update(render.status_line(s, mode, now))

    def _filter_label(self) -> str:
        node = self.store.get(self.filter) if self.filter else None
        return "main" if self.filter == MAIN else (node.kind if node else str(self.filter))

    def _refresh_input(self, in_chat: bool) -> None:
        inp, line = self.query_one(MessageInput), self.query_one("#sendstatus", Static)
        show = self.can_send and in_chat
        inp.display = show
        line.display = show
        if not show:
            return
        from rich.text import Text
        t = Text()
        sender = self.sender
        if sender is not None and sender.busy:
            t.append(f"{render.SPINNER[self.frame % len(render.SPINNER)]} Claude is working…  ", style="bold green")
            t.append("ctrl+k stops it", style="grey50")
        elif sender is not None and sender.last_error:
            t.append(f"✗ {sender.last_error}", style="bold red")
        else:
            t.append("i / enter: write to Claude", style="grey50")
        if self.permissions == "all":
            t.append("   ⚠ all permissions (no confirmations)", style="dark_orange")
        elif self.permissions == "plan":
            t.append("   plan mode: read-only", style="grey50")
        else:
            t.append("   accept-edits mode", style="grey50")
        line.update(t)

    def action_pause(self) -> None:
        if self.replay:
            self.replay.paused = not self.replay.paused

    def action_faster(self) -> None:
        if self.replay:
            self.replay.speed = min(self.replay.speed * 2, 64)

    def action_slower(self) -> None:
        if self.replay:
            self.replay.speed = max(self.replay.speed / 2, 0.25)


def make_live(session_jsonl=None, hooks: bool = True, projects_dir=None, **kw) -> AgentsTreeApp:
    """Live app; with no session it opens the picker on start. kw: permissions, claude_bin, can_send."""
    app = AgentsTreeApp(session=session_jsonl, hooks=hooks, projects_dir=projects_dir, **kw)
    if session_jsonl:
        app.tailers = [TranscriptTailer(session_jsonl)]
        if hooks:
            app.tailers.append(HookTailer(session_id=Path(session_jsonl).stem))
    return app
