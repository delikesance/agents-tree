import asyncio
import json

from agents_tree.model import AGENT_END, AGENT_START, MAIN, TURN, Event
from agents_tree.sources.transcript import TranscriptParser
from agents_tree.store import STALE_SECS, Store
from agents_tree.ui import render
from agents_tree.ui.app import AgentsTreeApp, make_live
from agents_tree.sources.replay import Replay


def store_with_mixed_agents():
    s = Store()
    t0 = 10_000
    for i, kind in enumerate(["done_one", "failed_one", "silent_one", "busy_one"]):
        s.apply(Event(t0, AGENT_START, kind, MAIN, {"kind": kind}))
    s.apply(Event(t0 + 5, AGENT_END, "done_one", data={"status": "done"}))
    s.apply(Event(t0 + 5, AGENT_END, "failed_one", data={"status": "failed"}))
    # "busy_one" keeps emitting turns; "silent_one" went quiet long ago.
    s.apply(Event(t0 + STALE_SECS + 50, TURN, "busy_one", data={"usage": {"output_tokens": 1}, "tool": "Bash"}))
    return s, t0 + STALE_SECS + 60


def test_active_view_only_lists_running_agents():
    s, now = store_with_mixed_agents()
    assert s.visible_children(MAIN, now, show_all=False) == ["busy_one"]
    assert s.visible_children(MAIN, now, show_all=True) == ["done_one", "failed_one", "silent_one", "busy_one"]


def test_running_child_keeps_its_finished_parent_visible():
    s = Store()
    s.apply(Event(1, AGENT_START, "parent", MAIN, {"kind": "dispatcher"}))
    s.apply(Event(2, AGENT_START, "child", "parent", {"kind": "worker"}))
    s.apply(Event(3, AGENT_END, "parent", data={"status": "done"}))
    assert s.visible_children(MAIN, 4) == ["parent"]
    assert s.visible_children("parent", 4) == ["child"]
    s.apply(Event(5, AGENT_END, "child", data={"status": "done"}))
    assert s.visible_children(MAIN, 6) == []


def test_main_goes_idle_but_stays_visible():
    s = Store()
    s.apply(Event(1000, TURN, MAIN, data={"model": "claude-sonnet-5-5", "usage": {"output_tokens": 1}}))
    assert s.state(s.nodes[MAIN], now=1010) == "running"
    assert s.state(s.nodes[MAIN], now=1000 + STALE_SECS + 1) == "stale"
    text = render.tree_view(s, 100, 0, now=1000 + 10 * STALE_SECS, show_all=False).plain
    assert "idle" in text and "no agent running" in text


def test_activity_tracks_last_tool_and_clears():
    p = TranscriptParser()
    turn_with_tool = {"type": "assistant", "timestamp": "2026-10-09T10:00:00Z", "message": {
        "model": "m", "usage": {"output_tokens": 1}, "content": [
            {"type": "tool_use", "id": "a", "name": "Read", "input": {}},
            {"type": "tool_use", "id": "b", "name": "mcp__jev__jev_which_file", "input": {}}]}}
    turn_text_only = {"type": "assistant", "timestamp": "2026-10-09T10:00:05Z", "message": {
        "model": "m", "usage": {"output_tokens": 1}, "content": [{"type": "text", "text": "done"}]}}
    s = Store()
    for d in (turn_with_tool, turn_text_only):
        evs = list(p.parse(d))
        for e in evs:
            s.apply(e)
        if d is turn_with_tool:
            assert s.nodes[MAIN].activity == "jev_which_file"   # last tool, server prefix stripped
    assert s.nodes[MAIN].activity == ""


def test_rendered_active_tree_hides_finished_agents_and_shows_tool():
    s, now = store_with_mixed_agents()
    active = render.tree_view(s, 140, 0, now, show_all=False).plain
    assert "busy_one" in active and "Bash" in active
    for gone in ("done_one", "failed_one", "silent_one"):
        assert gone not in active
    everything = render.tree_view(s, 200, 0, now, show_all=True).plain
    assert all(k in everything for k in ("done_one", "failed_one", "silent_one", "busy_one"))


def test_h_toggles_view_and_defaults_differ_for_live_and_replay(tmp_path):
    line = {"type": "assistant", "timestamp": "2026-10-09T10:00:00Z",
            "message": {"model": "claude-sonnet-5-5", "usage": {"output_tokens": 1}, "content": []}}
    f = tmp_path / "s.jsonl"
    f.write_text(json.dumps(line) + "\n")

    async def run():
        live = make_live(f, hooks=False)
        assert live.show_all is False
        async with live.run_test(size=(120, 30)) as pilot:
            await pilot.pause(0.3)
            await pilot.press("h")
            assert live.show_all is True
            await pilot.press("h")
            assert live.show_all is False
        rep = AgentsTreeApp(replay=Replay([Event(1, TURN, MAIN, data={"usage": {}})]))
        assert rep.show_all is True
    asyncio.run(run())
