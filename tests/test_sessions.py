import asyncio
import json
import os
import shutil
from pathlib import Path

from agents_tree.sources.live import HookTailer, list_sessions, session_title
from agents_tree.ui.app import make_live

FIX = Path(__file__).parent / "fixtures"


def make_projects(tmp_path):
    """Two projects: 'proj-a' (3 subagent-free copy of the fixture, older) and 'proj-b' (newer, one turn)."""
    a = tmp_path / "-home-user-proj-a"
    b = tmp_path / "-home-user-proj-b"
    a.mkdir(); b.mkdir()
    # Session A: the fixture (has worker + explorer subagents), with a user prompt prepended.
    prompt = {"type": "user", "message": {"content": [
        {"type": "text", "text": "<system-reminder>ctx</system-reminder>"},
        {"type": "text", "text": "Build the <b>login</b> page"}]}}
    (a / "aaaa1111.jsonl").write_text(json.dumps(prompt) + "\n" + (FIX / "session.jsonl").read_text())
    shutil.copytree(FIX / "session" / "subagents", a / "aaaa1111" / "subagents")
    # Session B: a single main turn.
    turn = {"type": "assistant", "timestamp": "2026-10-09T11:00:00Z", "effort": "low",
            "message": {"model": "claude-haiku-5-5", "usage": {"output_tokens": 7}, "content": []}}
    (b / "bbbb2222.jsonl").write_text(json.dumps({"type": "user", "message": {"content": "Fix the typo"}})
                                      + "\n" + json.dumps(turn) + "\n")
    os.utime(a / "aaaa1111.jsonl", (1000, 1000))
    os.utime(b / "bbbb2222.jsonl", (2000, 2000))
    return tmp_path


def test_list_sessions_sorted_with_titles(tmp_path):
    root = make_projects(tmp_path)
    got = list_sessions(root)
    assert [s.id for s in got] == ["bbbb2222", "aaaa1111"]
    assert got[0].title == "Fix the typo"
    assert got[1].title == "Build the login page"   # reminders and tags stripped
    assert got[1].subagents == 1 and got[0].subagents == 0


def test_session_title_missing_file(tmp_path):
    assert session_title(tmp_path / "nope.jsonl") == ""


def test_hook_tailer_filters_by_session(tmp_path):
    p = tmp_path / "ev.jsonl"
    lines = [{"ts": 1, "kind": "agent_start", "agent_id": "x", "session": "S1", "data": {"kind": "worker"}},
             {"ts": 2, "kind": "agent_start", "agent_id": "y", "session": "S2", "data": {"kind": "worker"}},
             {"ts": 3, "kind": "agent_start", "agent_id": "z", "data": {"kind": "worker"}}]
    p.write_text("\n".join(json.dumps(x) for x in lines) + "\n")
    assert [e.agent_id for e in HookTailer(p, session_id="S1").poll()] == ["x", "z"]


def test_picker_switches_session_and_rebuilds_tree(tmp_path):
    root = make_projects(tmp_path)

    async def run():
        app = make_live(None, hooks=False, projects_dir=root)
        async with app.run_test(size=(140, 40)) as pilot:
            await pilot.pause(0.3)
            assert app.session is None                  # picker is open on start
            await pilot.press("enter")                  # first row = newest = session B
            await pilot.pause(0.6)
            assert app.session.stem == "bbbb2222"
            assert [n.kind for n in app.store.nodes.values()] == ["main"]
            assert app.store.nodes["main"].effort == "low"

            await pilot.press("s")                      # reopen picker, pick session A
            await pilot.pause(0.3)
            await pilot.press("down", "enter")
            await pilot.pause(0.6)
            assert app.session.stem == "aaaa1111"
            kinds = sorted(n.kind for n in app.store.nodes.values())
            assert kinds == ["explorer", "main", "worker"]  # store was reset, B's state is gone
            assert app.store.nodes["main"].effort == "high"

            await pilot.press("s")                      # escape keeps the current session
            await pilot.pause(0.3)
            await pilot.press("escape")
            await pilot.pause(0.2)
            assert app.session.stem == "aaaa1111"
    asyncio.run(run())


def test_picker_filter(tmp_path):
    root = make_projects(tmp_path)

    async def run():
        app = make_live(None, hooks=False, projects_dir=root)
        async with app.run_test(size=(140, 40)) as pilot:
            await pilot.pause(0.3)
            from textual.widgets import DataTable, Input
            app.screen.query_one(Input).focus()
            await pilot.press(*"sq")                    # app bindings (s, q) must not fire while typing
            await pilot.pause(0.2)
            assert app.screen.query_one(Input).value == "sq" and app.is_running
            assert app.screen.query_one(DataTable).row_count == 0
            await pilot.press("backspace", "backspace")
            await pilot.press(*"login")
            await pilot.pause(0.2)
            assert app.screen.query_one(DataTable).row_count == 1
            await pilot.press("enter")                  # enter in the filter opens the matching session
            await pilot.pause(0.5)
            assert app.session.stem == "aaaa1111"
    asyncio.run(run())
