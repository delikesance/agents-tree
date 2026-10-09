import json
from pathlib import Path

from agents_tree.model import MAIN, MESSAGE, MSG_UPDATE, Event, Message
from agents_tree.sources.transcript import clean_text, read_session, tool_detail
from agents_tree.store import MAX_MESSAGES, Store


def line(**kw):
    kw.setdefault("timestamp", "2026-10-09T10:00:00Z")
    return json.dumps(kw)


def session(tmp_path, lines, sub=None):
    p = tmp_path / "proj" / "sess.jsonl"
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text("\n".join(lines) + "\n")
    if sub:
        d = p.with_suffix("") / "subagents"
        d.mkdir(parents=True)
        for name, (meta, sublines) in sub.items():
            (d / f"agent-{name}.jsonl").write_text("\n".join(sublines) + "\n")
            (d / f"agent-{name}.meta.json").write_text(json.dumps(meta))
    return p


def store_for(path):
    s = Store()
    for e in read_session(path):
        s.apply(e)
    return s


def usage():
    return {"output_tokens": 1}


def test_clean_text_removes_reminders_tags_and_formats_commands():
    assert clean_text("hello <system-reminder>secret ctx</system-reminder> world") == "hello  world"
    assert clean_text("<command-message>init</command-message>\n<command-name>/init</command-name>") == "/init"
    assert clean_text("<command-name>/design</command-name><command-args>make it</command-args>") == "/design make it"
    assert clean_text("a <b>bold</b> b") == "a bold b"


def test_tool_detail_is_short_and_specific():
    assert tool_detail("Bash", {"command": "pytest -q\nsecond line"}) == "pytest -q"
    assert tool_detail("Read", {"file_path": "/home/user/proj/pkg/mod.py"}) == "proj/pkg/mod.py"
    assert tool_detail("Grep", {"pattern": "TODO", "path": "src"}) == "TODO  in src"
    assert tool_detail("mcp__x__do", {"a": 1, "b": "first string"}) == "first string"


def test_user_system_and_interrupt_roles(tmp_path):
    p = session(tmp_path, [
        line(type="user", uuid="u1", origin={"kind": "human"}, turnOrigin="human",
             message={"role": "user", "content": "<command-name>/init</command-name>"}),
        line(type="user", uuid="u2", isMeta=True, message={"role": "user", "content": [
            {"type": "text", "text": "Please analyze this codebase"}]}),
        line(type="user", uuid="u3", origin={"kind": "human"}, message={"role": "user", "content": [
            {"type": "text", "text": "Look at this <system-reminder>noise</system-reminder>"},
            {"type": "image", "source": {}}]}),
        line(type="user", uuid="u4", message={"role": "user", "content": "[Request interrupted by user]"}),
        line(type="user", uuid="u5", isMeta=True, message={"role": "user", "content": "Stop hook feedback: x"}),
    ])
    got = [(m.role, m.text) for m in store_for(p).messages]
    assert got == [("user", "/init"), ("system", "Please analyze this codebase"),
                   ("user", "Look at this  ▣ image"), ("system", "[Request interrupted by user]"),
                   ("system", "Stop hook feedback: x")]


def test_assistant_text_and_tool_pairing_with_duration_and_status(tmp_path):
    p = session(tmp_path, [
        line(type="assistant", uuid="a1", timestamp="2026-10-09T10:00:00Z", message={
            "model": "claude-sonnet-5-5", "usage": usage(), "content": [
                {"type": "thinking", "thinking": ""},
                {"type": "text", "text": "Running the tests."},
                {"type": "tool_use", "id": "t1", "name": "Bash", "input": {"command": "pytest -q"}},
                {"type": "tool_use", "id": "t2", "name": "Read", "input": {"file_path": "/a/b/c.py"}}]}),
        line(type="user", timestamp="2026-10-09T10:00:03Z", message={"content": [
            {"type": "tool_result", "tool_use_id": "t1", "content": "1 failed", "is_error": True}]}),
        line(type="user", timestamp="2026-10-09T10:00:01Z", message={"content": [
            {"type": "tool_result", "tool_use_id": "t2", "content": "ok"}]}),
    ])
    msgs = {m.id: m for m in store_for(p).messages}
    assert msgs["a1:1"].role == "assistant" and msgs["a1:1"].model == "claude-sonnet-5-5"
    assert (msgs["t1"].tool, msgs["t1"].detail, msgs["t1"].status, msgs["t1"].duration) == ("Bash", "pytest -q", "error", 3.0)
    assert (msgs["t2"].status, msgs["t2"].duration, msgs["t2"].detail) == ("ok", 1.0, "a/b/c.py")
    assert msgs["t1"].rev == 1                              # one update -> views re-render it


def test_unanswered_tool_stays_running(tmp_path):
    p = session(tmp_path, [line(type="assistant", uuid="a", message={"model": "m", "usage": usage(), "content": [
        {"type": "tool_use", "id": "t", "name": "Bash", "input": {"command": "sleep 99"}}]})])
    assert store_for(p).messages[0].status == "running"


def test_delegation_report_subagent_prompt_not_duplicated(tmp_path):
    main = [
        line(type="assistant", uuid="a", timestamp="2026-10-09T10:00:00Z", message={
            "model": "claude-sonnet-5-5", "usage": usage(), "content": [
                {"type": "tool_use", "id": "ag1", "name": "Agent", "input": {
                    "subagent_type": "explorer", "description": "map the repo", "prompt": "List the files."}}]}),
        line(type="user", timestamp="2026-10-09T10:00:08Z", toolUseResult={"agentId": "a99"}, message={"content": [
            {"type": "tool_result", "tool_use_id": "ag1", "content": [
                {"type": "text", "text": "Found 3 files.\nagentId: a99"}]}]}),
    ]
    sub = {"a99": ({"toolUseId": "ag1"}, [
        line(type="user", isSidechain=True, uuid="s0", message={"role": "user", "content": "List the files."}),
        line(type="assistant", isSidechain=True, uuid="s1", timestamp="2026-10-09T10:00:02Z", message={
            "model": "claude-haiku-5-5", "usage": usage(), "content": [
                {"type": "text", "text": "Looking."},
                {"type": "tool_use", "id": "st1", "name": "Bash", "input": {"command": "ls"}},
                {"type": "tool_use", "id": "hb", "name": "SubagentHandback", "input": {}}]}),
    ])}
    s = store_for(session(tmp_path, main, sub))
    roles = [(m.role, m.agent_id, m.id) for m in s.messages]
    assert ("delegation", MAIN, "deleg:ag1") in roles and ("report", MAIN, "rep:ag1") in roles
    assert not any(m.id == "s0" for m in s.messages)               # the prompt is the delegation's body
    assert not any(m.tool == "SubagentHandback" for m in s.messages)
    deleg = next(m for m in s.messages if m.role == "delegation")
    assert (deleg.kind, deleg.desc, deleg.text, deleg.target) == ("explorer", "map the repo", "List the files.", "ag1")
    assert deleg.status == "done" and deleg.duration == 8.0
    report = next(m for m in s.messages if m.role == "report")
    assert report.text == "Found 3 files." and report.status == "done"
    sub_tool = next(m for m in s.messages if m.id == "st1")
    assert s.get(sub_tool.agent_id).id == "ag1"                    # subagent messages resolve to its node


def test_failed_agent_report(tmp_path):
    p = session(tmp_path, [
        line(type="assistant", uuid="a", message={"model": "m", "usage": usage(), "content": [
            {"type": "tool_use", "id": "ag", "name": "Task", "input": {"subagent_type": "worker", "prompt": "x"}}]}),
        line(type="user", message={"content": [
            {"type": "tool_result", "tool_use_id": "ag", "is_error": True, "content": "boom"}]}),
    ])
    rep = next(m for m in store_for(p).messages if m.role == "report")
    assert rep.status == "failed" and rep.text == "boom"


def test_store_dedupes_and_caps_messages():
    s = Store()
    m = Message("same", 1, MAIN, "assistant", "x")
    s.apply(Event(1, MESSAGE, MAIN, data={"message": m}))
    s.apply(Event(1, MESSAGE, MAIN, data={"message": Message("same", 1, MAIN, "assistant", "dup")}))
    assert len(s.messages) == 1 and s.messages[0].text == "x"
    s.apply(Event(2, MSG_UPDATE, MAIN, data={"id": "nope", "status": "ok"}))      # unknown id: ignored
    for i in range(MAX_MESSAGES + 50):
        s.apply(Event(i, MESSAGE, MAIN, data={"message": Message(f"m{i}", i, MAIN, "assistant", "t")}))
    assert len(s.messages) == MAX_MESSAGES and s.messages[-1].id == f"m{MAX_MESSAGES + 49}"
    s.apply(Event(9, MSG_UPDATE, MAIN, data={"id": "m0", "status": "ok"}))        # trimmed id: ignored


def test_bash_heredoc_shows_what_it_does_not_just_the_interpreter():
    assert tool_detail("Bash", {"command": "python3 - <<'E'\nimport json\nprint(1)\nE"}) == "python3 - <<'E' ⏎ import json"
    assert tool_detail("Bash", {"command": "ls -la\npwd"}) == "ls -la"
