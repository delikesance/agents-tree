import json

import pytest

from agents_tree.composition import Composition, analyze


def req(mid, ts, content, *, read=0, fresh=0, write=0, model="claude-sonnet-5-5"):
    return json.dumps({"type": "assistant", "timestamp": ts, "uuid": mid, "message": {
        "id": mid, "model": model, "content": content,
        "usage": {"input_tokens": fresh, "cache_read_input_tokens": read, "cache_creation_input_tokens": write,
                  "output_tokens": 1}}})


def user(content):
    return json.dumps({"type": "user", "timestamp": "2026-10-09T10:00:00Z", "message": {"role": "user", "content": content}})


def result(tid, text):
    return user([{"type": "tool_result", "tool_use_id": tid, "content": text}])


def write_session(tmp_path, lines):
    p = tmp_path / "s.jsonl"
    p.write_text("\n".join(lines) + "\n")
    return p


def test_volume_is_size_times_the_requests_that_come_after(tmp_path):
    # 4 requests. Request 1 calls Bash (input 40 chars = 10 tokens ... see below); its 400-char output
    # (100 tokens) is re-sent by requests 2, 3 and 4 -> volume 300.
    cmd = {"command": "x" * 25}                      # json.dumps -> 40 chars -> 10 tokens
    p = write_session(tmp_path, [
        req("m1", "2026-10-09T10:00:00Z", [{"type": "tool_use", "id": "t1", "name": "Bash", "input": cmd}], fresh=1000),
        result("t1", "y" * 400),
        req("m2", "2026-10-09T10:00:01Z", [{"type": "text", "text": "a" * 80}], read=1110),      # 20 tokens of text
        user("u" * 40),                                                                         # 10 tokens, 1 later request
        req("m3", "2026-10-09T10:00:02Z", [{"type": "text", "text": "done"}], read=1250),
        req("m4", "2026-10-09T10:00:03Z", [{"type": "text", "text": "bye"}], read=1260),
    ])
    c = analyze(p)
    assert c.turns == 4 and c.prompt_tokens == 1000 + 1110 + 1250 + 1260
    assert c.baseline_tokens == 1000 and c.baseline == 1000 * 4
    assert c.outputs["Bash"].n == 1 and c.outputs["Bash"].tokens == 100
    assert c.outputs["Bash"].volume == 100 * 3                      # re-sent by requests 2, 3, 4
    assert c.inputs["Bash"].tokens == 10 and c.inputs["Bash"].volume == 10 * 3
    assert c.assistant_text == pytest.approx(20 * 2 + 1 * 1, abs=1e-9)         # m2 (20 tok): 2 later; m3 ("done"): 1
    assert c.user_text == 10 * 2                                    # after m2: requests 3 and 4
    assert c.other >= 0
    assert c.share(c.baseline) == pytest.approx(4000 / c.prompt_tokens)


def test_repeated_usage_lines_of_one_request_are_one_request(tmp_path):
    one = req("m1", "2026-10-09T10:00:00Z", [{"type": "text", "text": "a"}], fresh=100)
    two = req("m1", "2026-10-09T10:00:01Z", [{"type": "tool_use", "id": "t", "name": "Read", "input": {}}], fresh=100)
    p = write_session(tmp_path, [one, two, result("t", "z" * 40),
                                 req("m2", "2026-10-09T10:00:02Z", [{"type": "text", "text": "b"}], read=150)])
    c = analyze(p)
    assert c.turns == 2 and c.prompt_tokens == 250
    assert c.outputs["Read"].volume == 10 * 1                       # one request after it


def test_cost_share_and_what_if(tmp_path):
    p = write_session(tmp_path, [
        req("m1", "2026-10-09T10:00:00Z", [{"type": "tool_use", "id": "t", "name": "Bash", "input": {}}], fresh=1_000_000),
        result("t", "q" * 400_000),                                  # 100k tokens
        req("m2", "2026-10-09T10:00:01Z", [{"type": "text", "text": "ok"}], read=1_100_000),
    ])
    c = analyze(p)
    assert c.input_cost == pytest.approx(1_000_000 * 2 / 1e6 + 1_100_000 * 0.20 / 1e6)     # Sonnet 5.5 rates
    assert c.share(c.outputs_volume) == pytest.approx(100_000 / 2_100_000)
    share, usd = c.what_if_compress_outputs(0.5)
    assert share == pytest.approx(0.5 * 100_000 / 2_100_000) and usd == pytest.approx(share * c.input_cost)


def test_subagent_files_are_separate_conversations(tmp_path):
    main = write_session(tmp_path, [req("m1", "2026-10-09T10:00:00Z", [{"type": "text", "text": "x"}], fresh=500)])
    sub = tmp_path / "s" / "subagents"
    sub.mkdir(parents=True)
    (sub / "agent-a1.jsonl").write_text(req("s1", "2026-10-09T10:00:00Z", [{"type": "text", "text": "y"}], fresh=300) + "\n")
    both, only_main = analyze(main), analyze(main, subagents=False)
    assert both.turns == 2 and both.prompt_tokens == 800 and only_main.prompt_tokens == 500


def test_empty_or_unreadable_sessions_do_not_crash(tmp_path):
    p = tmp_path / "e.jsonl"
    p.write_text("not json\n{}\n")
    c = analyze(p)
    assert c.turns == 0 and c.share(10) == 0 and c.what_if_compress_outputs() == (0.0, 0.0)
    assert isinstance(Composition().categories(), list)
    assert analyze(tmp_path / "missing.jsonl").turns == 0


def test_cli_context_command(tmp_path, capsys):
    from agents_tree.cli import main
    p = write_session(tmp_path, [
        req("m1", "2026-10-09T10:00:00Z", [{"type": "tool_use", "id": "t", "name": "Bash", "input": {}}], fresh=1000),
        result("t", "q" * 4000),
        req("m2", "2026-10-09T10:00:01Z", [{"type": "text", "text": "ok"}], read=2000),
    ])
    assert main(["context", str(p)]) == 0
    out = capsys.readouterr().out
    assert "2 requests" in out and "tool outputs" in out and "Bash (1 calls" in out and "Shrinking every tool output" in out
    assert main(["context", str(tmp_path / "nope.jsonl")]) == 1
