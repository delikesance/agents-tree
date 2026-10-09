from pathlib import Path

from agents_tree.model import MAIN, Event, AGENT_START, AGENT_END
from agents_tree.sources.replay import Replay
from agents_tree.sources.transcript import read_session
from agents_tree.store import Store

FIX = Path(__file__).parent / "fixtures" / "session.jsonl"


def build() -> Store:
    s = Store()
    for ev in read_session(FIX):
        s.apply(ev)
    return s


def test_tree_shape_and_status():
    s = build()
    subs = [s.nodes[c] for c in s.nodes[MAIN].children]
    assert [(n.kind, n.status) for n in subs] == [("explorer", "done"), ("worker", "failed")]
    assert s.running_subagents() == (0, 2)


def test_sidechain_turns_attach_via_alias():
    s = build()
    explorer = s.get("a1")
    assert explorer.kind == "explorer" and explorer.turns == 2
    assert explorer.tokens_in == 200 and explorer.tokens_out == 400
    assert explorer.cost > 0


def test_main_stats_advisor_jev():
    s = build()
    main = s.nodes[MAIN]
    assert main.effort == "high" and main.model == "claude-sonnet-5-5"
    assert s.advisor.calls == 1 and s.advisor.last_advice == "verify auth claims"
    assert s.advisor.model == "claude-opus-5-5"
    assert s.jev.forks == 1
    assert s.jev.avg_confidence("jev_which_file") == 0.86


def test_hook_start_merges_with_transcript_node():
    s = Store()
    s.apply(Event(1, AGENT_START, "tool1", MAIN, {"kind": "worker"}))
    s.apply(Event(2, AGENT_START, "hook-agent", data={"kind": "worker", "match_kind": "worker"}))
    assert len(s.nodes) == 2  # main + one worker
    s.apply(Event(3, AGENT_END, "hook-agent", data={"status": "done"}))
    assert s.nodes["tool1"].status == "done"


def test_replay_collapses_gaps_and_pauses():
    evs = read_session(FIX)
    r = Replay(evs, speed=1.0, max_gap=1.0)
    got = r.tick(0.0)
    assert got and not r.done
    r.paused = True
    assert r.tick(100) == []
    r.paused = False
    for _ in range(100):
        r.tick(1.0)
    assert r.done


def test_meta_alias_binds_before_node_exists(tmp_path):
    from agents_tree.model import ALIAS, TURN
    s = Store()
    s.apply(Event(0, ALIAS, "tool1", data={"alias": "agentX"}))  # sorted first by ts
    s.apply(Event(1, AGENT_START, "tool1", MAIN, {"kind": "worker"}))
    s.apply(Event(2, TURN, "agentX", data={"model": "claude-haiku-5-5", "usage": {"output_tokens": 5}, "sidechain": True}))
    assert s.nodes["tool1"].turns == 1


def test_tailer_incremental(tmp_path):
    import json
    from agents_tree.sources.live import TranscriptTailer
    p = tmp_path / "s.jsonl"
    line = {"type": "assistant", "timestamp": "2026-10-09T10:00:00Z",
            "message": {"model": "claude-sonnet-5-5", "usage": {"output_tokens": 1}, "content": []}}
    p.write_text(json.dumps(line) + "\n" + json.dumps(line)[:20])  # second line is partial
    t = TranscriptTailer(p)
    assert len(t.poll()) == 1
    assert t.poll() == []
    with open(p, "a") as f:
        f.write(json.dumps(line)[20:] + "\n")
    assert len(t.poll()) == 1


def test_cli_rejects_missing_transcript(capsys):
    from agents_tree.cli import main
    assert main(["live", "/nonexistent.jsonl"]) == 1
    assert main(["replay", "/nonexistent.jsonl"]) == 1
    assert "not found" in capsys.readouterr().err


def test_silent_agent_is_not_shown_as_running():
    from agents_tree.model import TURN
    from agents_tree.store import STALE_SECS
    s = Store()
    s.apply(Event(1000, AGENT_START, "w", MAIN, {"kind": "worker"}))
    w = s.nodes["w"]
    assert s.state(w) == "running"
    assert s.state(w, now=1000 + STALE_SECS + 1) == "stale"       # live clock moved on, no events
    s.apply(Event(1000 + STALE_SECS + 5, TURN, "w", data={"usage": {"output_tokens": 1}}))
    assert s.state(w) == "running"                                # activity revives it
    assert s.running_subagents(now=1000 + 10 * STALE_SECS) == (0, 1)
    s.apply(Event(1000 + 11 * STALE_SECS, AGENT_END, "w", data={"status": "done"}))
    assert s.state(w, now=10**10) == "done"                       # finished agents never go stale


def test_panels_hidden_until_they_really_happen():
    import asyncio
    from agents_tree.ui.app import AgentsTreeApp
    from agents_tree.sources.replay import Replay

    async def run():
        app = AgentsTreeApp(replay=Replay([Event(1, AGENT_START, "t", MAIN, {"kind": "worker"})], speed=64))
        async with app.run_test(size=(120, 40)) as pilot:
            await pilot.pause(0.5)
            assert app.query_one("#left").display                 # the agent rail is always there
            assert not app.query_one("#advisor").display          # advisor/JEV only after a call
            assert not app.query_one("#jev").display
            from agents_tree.model import ADVISOR
            app.store.apply(Event(2, ADVISOR, MAIN, data={}))
            app.refresh_views()
            assert app.query_one("#advisor").display and not app.query_one("#jev").display
    asyncio.run(run())
