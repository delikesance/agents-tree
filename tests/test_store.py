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
