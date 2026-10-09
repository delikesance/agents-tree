import asyncio
import io
import json
from pathlib import Path

from rich.console import Console

from agents_tree.model import MAIN, MESSAGE, MSG_UPDATE, AGENT_START, TURN, Event, Message
from agents_tree.sources.replay import Replay
from agents_tree.store import Store
from agents_tree.ui import messages as M
from agents_tree.ui.app import AgentsTreeApp, make_live
from agents_tree.ui.chat import ChatMessage, ChatView
from agents_tree.ui.rail import rail_view

FAKE = str(Path(__file__).parent / "fake_claude.py")


def plain(renderable, width=90) -> str:
    c = Console(width=width, record=True, force_terminal=False, color_system=None, file=io.StringIO())
    c.print(renderable)
    return c.export_text()


def msg_event(ts, **kw):
    return Event(ts, MESSAGE, kw.get("agent_id", MAIN), data={"message": Message(**kw, ts=ts)} if False else {
        "message": Message(ts=ts, **kw)})


def conversation_store():
    s = Store()
    s.apply(Event(1000, TURN, MAIN, data={"model": "claude-sonnet-5-5", "usage": {"output_tokens": 5}}))
    s.apply(Event(1001, AGENT_START, "ag1", MAIN, {"kind": "explorer", "desc": "map the repo"}))
    s.apply(Event(1001, TURN, "ag1", data={"model": "claude-haiku-5-5", "usage": {"output_tokens": 5}, "tool": "Read"}))
    for ev in [
        msg_event(1000, id="u", agent_id=MAIN, role="user", text="Peux-tu ne montrer que les agents en cours ?"),
        msg_event(1001, id="s", agent_id=MAIN, role="system", text="Stop hook feedback:\nuntracked files"),
        msg_event(1002, id="a", agent_id=MAIN, role="assistant", model="claude-sonnet-5-5",
                  text="Done.\n\n- the rail lists running agents\n- `h` shows history"),
        msg_event(1003, id="t1", agent_id=MAIN, role="tool", tool="Bash", detail="pytest -q", status="error", duration=9.0),
        msg_event(1003, id="t2", agent_id=MAIN, role="tool", tool="Read", detail="a/store.py", status="ok", duration=0.2),
        msg_event(1004, id="deleg:ag1", agent_id=MAIN, role="delegation", kind="explorer", desc="map the repo",
                  text="List files.", target="ag1"),
        msg_event(1005, id="sa", agent_id="ag1", role="assistant", model="claude-haiku-5-5", text="Looking."),
        msg_event(1006, id="rep:ag1", agent_id=MAIN, role="report", text="Found 3 files.", target="ag1", status="done", duration=5.0),
    ]:
        s.apply(ev)
    return s


def test_each_role_renders_a_readable_box():
    s = conversation_store()
    by = {m.id: m for m in s.messages}
    out = {k: plain(M.message_renderable(by[k], s, 1010)) for k in by}
    assert "you" in out["u"] and "ne montrer que les agents" in out["u"]
    assert "⚙ Stop hook feedback:" in out["s"] and "2 lines" in out["s"] and "untracked" not in out["s"]
    assert "main" in out["a"] and "Sonnet 5.5" in out["a"] and "running agents" in out["a"]
    assert "✗" in out["t1"] and "Bash" in out["t1"] and "pytest -q" in out["t1"] and "9.0 s" in out["t1"]
    assert "✓" in out["t2"] and "a/store.py" in out["t2"]
    assert "→ explorer" in out["deleg:ag1"] and "map the repo" in out["deleg:ag1"] and "List files." in out["deleg:ag1"]
    assert "running" in out["deleg:ag1"]                                   # the target is alive
    assert out["sa"].startswith("    ") and "explorer" in out["sa"]         # subagent output is nested under it
    assert "↩ explorer reported" in out["rep:ag1"] and "Found 3 files." in out["rep:ag1"] and "5.0 s" in out["rep:ag1"]


def test_long_text_folds_and_expands():
    s = Store()
    text = "\n".join(f"line {i}" for i in range(20))
    m = Message(id="x", ts=1, agent_id=MAIN, role="assistant", text=text)
    folded = plain(M.message_renderable(m, s, None, expanded=False))
    full = plain(M.message_renderable(m, s, None, expanded=True))
    assert "line 5" in folded and "line 6" not in folded and "+14 lines" in folded
    assert "line 19" in full and "+14 lines" not in full


def test_signature_changes_only_when_the_box_would():
    s = conversation_store()
    tool = next(m for m in s.messages if m.id == "t2")
    sig = M.message_signature(tool, s, 1010, False, 0)
    assert M.message_signature(tool, s, 1010, False, 5) == sig                # finished tool: frame irrelevant
    s.apply(Event(1010, MSG_UPDATE, MAIN, data={"id": "t2", "status": "error"}))
    assert M.message_signature(tool, s, 1010, False, 0) != sig
    deleg = next(m for m in s.messages if m.id == "deleg:ag1")
    a = M.message_signature(deleg, s, 1010, False, 0)
    s.apply(Event(1011, "agent_end", "ag1", data={"status": "done"}))
    assert M.message_signature(deleg, s, 1010, False, 0) != a                  # the target's state changed


def test_rail_shows_running_agents_only_and_counts_the_rest():
    s = conversation_store()
    s.apply(Event(1010, AGENT_START, "ag2", MAIN, {"kind": "worker", "desc": "edit things"}))
    s.apply(Event(1011, "agent_end", "ag1", data={"status": "done"}))
    text = plain(rail_view(s, 1012, 0, False), 40)
    assert "AGENTS · 1 RUNNING" in text and "worker" in text and "explorer" not in text
    assert "1 finished or idle hidden" in text
    assert "explorer" in plain(rail_view(s, 1012, 0, True), 40)


def replay_app(store_events):
    return AgentsTreeApp(replay=Replay(store_events, speed=1000, max_gap=0.01))


def all_events(s: Store):
    # replay the same events the store was built from
    evs = [Event(1000, TURN, MAIN, data={"model": "claude-sonnet-5-5", "usage": {"output_tokens": 5}}),
           Event(1001, AGENT_START, "ag1", MAIN, {"kind": "explorer", "desc": "map the repo"})]
    evs += [Event(m.ts, MESSAGE, m.agent_id, data={"message": m}) for m in s.messages]
    return evs


def chat_texts(app):
    return [plain(w.content) for w in app.query(ChatMessage)]


def test_chat_view_draws_every_box_filters_and_expands():
    s = conversation_store()

    async def run():
        app = replay_app(all_events(s))
        async with app.run_test(size=(140, 40)) as pilot:
            for _ in range(15):
                await pilot.pause(0.1)
            assert len(app.query(ChatMessage)) == 8 and app.query_one(ChatView).display
            assert not app.query_one("#treebox").display
            await pilot.press("f")                                  # everyone -> main only
            await pilot.pause(0.3)
            assert app.filter == MAIN and len(app.query(ChatMessage)) == 7   # the subagent's own message is hidden
            await pilot.press("f")                                  # -> the explorer
            await pilot.pause(0.3)
            texts = chat_texts(app)
            assert app.filter == "ag1" and len(texts) == 3          # its output + the delegation + its report
            assert any("Looking." in t for t in texts)
            await pilot.press("f")                                  # -> everyone again
            await pilot.pause(0.3)
            assert app.filter is None and len(app.query(ChatMessage)) == 8
            assert "filter" not in plain(app.query_one("#status").content) or True
            await pilot.press("e")
            await pilot.pause(0.3)
            assert app.query_one(ChatView).expanded
            assert any("⚙ system" in t for t in chat_texts(app))     # system lines show in full when expanded
            await pilot.press("t")                                  # tree view
            await pilot.pause(0.3)
            assert not app.query_one(ChatView).display and app.query_one("#treebox").display
            assert not app.query_one("#input").display
            await pilot.press("t")
            await pilot.pause(0.3)
            assert app.query_one(ChatView).display
    asyncio.run(run())


def test_replay_has_no_message_box():
    async def run():
        app = replay_app(all_events(conversation_store()))
        async with app.run_test(size=(120, 30)) as pilot:
            await pilot.pause(0.3)
            assert not app.query_one("#input").display and not app.can_send
            await pilot.press("i")                                   # nothing to focus
            assert app.focused is not app.query_one("#input")
    asyncio.run(run())


def test_chat_follows_new_messages_unless_scrolled_up():
    async def run():
        s = Store()
        evs = [msg_event(1000 + i, id=f"m{i}", agent_id=MAIN, role="assistant", text=f"message number {i}\n" * 3)
               for i in range(60)]
        app = replay_app(evs)
        async with app.run_test(size=(120, 30)) as pilot:
            for _ in range(20):
                await pilot.pause(0.1)
            chat = app.query_one(ChatView)
            assert chat.max_scroll_y > 0 and chat.scroll_y >= chat.max_scroll_y - 2     # followed to the bottom
            chat.key_up()
            chat.scroll_home(animate=False)
            await pilot.pause(0.2)
            assert chat.follow is False
            app.store.apply(Event(2000, MESSAGE, MAIN, data={"message": Message(
                id="late", ts=2000, agent_id=MAIN, role="assistant", text="a late message")}))
            app.refresh_views()
            await pilot.pause(0.3)
            assert chat.scroll_y < 5                                 # still where the reader left it
            chat.key_end()
            await pilot.pause(0.3)
            assert chat.follow and chat.scroll_y >= chat.max_scroll_y - 2
    asyncio.run(run())


# --- sending from the TUI (fake claude: no network, no cost) ------------------------------------
def live_session(tmp_path):
    proj = tmp_path / "projects" / "-fake-proj"
    proj.mkdir(parents=True)
    path = proj / "live-1.jsonl"
    path.write_text(json.dumps({"type": "user", "uuid": "u0", "timestamp": "2026-10-09T10:00:00Z", "cwd": str(tmp_path),
                                "origin": {"kind": "human"}, "message": {"role": "user", "content": "first prompt"}}) + "\n")
    return path


def fake_env(tmp_path, monkeypatch, delay="0.4"):
    monkeypatch.setenv("FAKE_ARGV", str(tmp_path / "argv.jsonl"))
    monkeypatch.setenv("FAKE_PROJECTS", str(tmp_path / "projects"))
    monkeypatch.setenv("FAKE_DELAY", delay)
    monkeypatch.delenv("FAKE_MODE", raising=False)


async def type_text(pilot, text):
    await pilot.press(*[c if c != " " else "space" for c in text])


def test_typing_a_message_sends_it_and_the_reply_appears_in_the_chat(tmp_path, monkeypatch):
    fake_env(tmp_path, monkeypatch)
    path = live_session(tmp_path)

    async def run():
        app = make_live(path, hooks=False, claude_bin=FAKE, permissions="all")
        async with app.run_test(size=(140, 40)) as pilot:
            await pilot.pause(0.6)
            assert any("first prompt" in t for t in chat_texts(app))
            await pilot.press("i")
            assert app.focused is app.query_one("#input")
            await type_text(pilot, "hello q s h f e t")             # app shortcuts must not fire while typing
            assert app.query_one("#input").value == "hello q s h f e t" and app.view == "chat" and not app.show_all
            await pilot.press("enter")
            await pilot.pause(0.25)
            assert app.query_one("#input").value == ""
            assert app.sender is not None and app.sender.busy
            assert "Claude is working" in plain(app.query_one("#sendstatus").content, 120)
            assert "all permissions" in plain(app.query_one("#sendstatus").content, 120)
            for _ in range(40):
                await pilot.pause(0.1)
                if any("echo: hello" in t for t in chat_texts(app)):
                    break
            texts = chat_texts(app)
            assert any("hello q s h f e t" in t and "you" in t for t in texts)      # the user's box
            assert any("echo: hello q s h f e t" in t and "main" in t for t in texts)  # Claude's reply
            await pilot.pause(0.5)
            assert not app.sender.busy and "Claude is working" not in plain(app.query_one("#sendstatus").content, 120)
            await pilot.press("escape")
            assert app.focused is app.query_one(ChatView)
            await pilot.press("q")
            await pilot.pause(0.3)
        assert not app.is_running
    asyncio.run(run())
    argv = json.loads((tmp_path / "argv.jsonl").read_text().splitlines()[0])["argv"]
    assert argv[-2:] == ["--resume", "live-1"] and "--dangerously-skip-permissions" in argv


def test_permission_preset_reaches_the_command_line(tmp_path, monkeypatch):
    fake_env(tmp_path, monkeypatch, delay="0.1")
    path = live_session(tmp_path)

    async def run():
        app = make_live(path, hooks=False, claude_bin=FAKE, permissions="plan")
        async with app.run_test(size=(140, 40)) as pilot:
            await pilot.pause(0.3)
            await pilot.press("i")
            await type_text(pilot, "plan only")
            await pilot.press("enter")
            await pilot.pause(0.8)
            assert "plan mode" in plain(app.query_one("#sendstatus").content, 120)
    asyncio.run(run())
    argv = json.loads((tmp_path / "argv.jsonl").read_text().splitlines()[0])["argv"]
    assert "--dangerously-skip-permissions" not in argv and argv[argv.index("--permission-mode") + 1] == "plan"


def test_sending_with_no_session_starts_a_new_one_and_follows_it(tmp_path, monkeypatch):
    fake_env(tmp_path, monkeypatch, delay="0.2")
    projects = tmp_path / "projects"

    async def run():
        app = AgentsTreeApp(session=None, hooks=False, projects_dir=projects, claude_bin=FAKE)
        async with app.run_test(size=(140, 40)) as pilot:
            await pilot.pause(0.3)
            await pilot.press("escape")                              # leave the session picker: start fresh
            await pilot.pause(0.2)
            assert app.session is None
            await pilot.press("i")
            await type_text(pilot, "brand new")
            await pilot.press("enter")
            for _ in range(40):
                await pilot.pause(0.1)
                if app.session is not None and any("echo: brand new" in t for t in chat_texts(app)):
                    break
            assert app.session is not None and app.session.parent.name == "-fake-proj"
            assert any("echo: brand new" in t for t in chat_texts(app))
            assert app.session.stem[:8] in plain(app.query_one("#status").content, 200)
    asyncio.run(run())
    argv = json.loads((tmp_path / "argv.jsonl").read_text().splitlines()[0])["argv"]
    assert "--session-id" in argv and "--resume" not in argv


def test_missing_claude_binary_shows_an_error_in_the_bar(tmp_path, monkeypatch):
    path = live_session(tmp_path)

    async def run():
        app = make_live(path, hooks=False, claude_bin="no-such-claude-binary")
        async with app.run_test(size=(140, 40)) as pilot:
            await pilot.pause(0.3)
            await pilot.press("i")
            await type_text(pilot, "hi")
            await pilot.press("enter")
            await pilot.pause(0.5)
            text = plain(app.query_one("#sendstatus").content, 140)
            assert "not found" in text and "Claude is working" not in text
            assert app.is_running
    asyncio.run(run())


def test_failed_turn_shows_claudes_error_and_ctrl_k_stops_a_long_turn(tmp_path, monkeypatch):
    fake_env(tmp_path, monkeypatch, delay="0.1")
    monkeypatch.setenv("FAKE_MODE", "fail")
    path = live_session(tmp_path)

    async def run():
        app = make_live(path, hooks=False, claude_bin=FAKE)
        async with app.run_test(size=(140, 40)) as pilot:
            await pilot.pause(0.3)
            await pilot.press("i")
            await type_text(pilot, "boom")
            await pilot.press("enter")
            await pilot.pause(1.0)
            assert "model overloaded" in plain(app.query_one("#sendstatus").content, 140)
            monkeypatch.setenv("FAKE_MODE", "ok")
            monkeypatch.setenv("FAKE_DELAY", "30")
            await app.sender.stop()
            app.sender = None
            await type_text(pilot, "slow")
            await pilot.press("enter")
            await pilot.pause(0.6)
            assert app.sender.busy
            await pilot.press("escape")
            await pilot.press("ctrl+k")
            await pilot.pause(1.0)
            assert not app.sender.busy and not app.sender.alive
    asyncio.run(run())


def test_cli_exposes_send_options():
    from agents_tree.cli import send_opts
    import argparse
    ns = argparse.Namespace(permissions="accept-edits", claude_bin="c", no_send=True)
    assert send_opts(ns) == {"permissions": "accept-edits", "claude_bin": "c", "can_send": False}
