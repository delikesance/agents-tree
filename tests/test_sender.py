import asyncio
import json
import os
import sys
from pathlib import Path

import pytest

from agents_tree.sender import ClaudeSender, build_command, clean_env, find_transcript, user_line

FAKE = str(Path(__file__).parent / "fake_claude.py")


@pytest.fixture
def fake_env(tmp_path, monkeypatch):
    monkeypatch.setenv("FAKE_ARGV", str(tmp_path / "argv.jsonl"))
    monkeypatch.setenv("FAKE_PROJECTS", str(tmp_path / "projects"))
    monkeypatch.setenv("FAKE_DELAY", "0.2")
    monkeypatch.delenv("FAKE_MODE", raising=False)
    return tmp_path


def calls(tmp_path):
    return [json.loads(x) for x in (tmp_path / "argv.jsonl").read_text().splitlines()]


async def until(cond, timeout=8.0):
    end = asyncio.get_event_loop().time() + timeout
    while not cond():
        assert asyncio.get_event_loop().time() < end, "timed out"
        await asyncio.sleep(0.05)


def test_build_command_flags_for_each_permission_preset():
    base = build_command("claude", "S1", True, "all")
    assert base[:2] == ["claude", "-p"] and "--dangerously-skip-permissions" in base
    assert ["--input-format", "stream-json"] == base[2:4] and "--verbose" in base
    assert base[-2:] == ["--resume", "S1"]
    c = build_command("claude", "S2", False, "plan", model="haiku")
    assert "--dangerously-skip-permissions" not in c
    assert c[c.index("--permission-mode") + 1] == "plan" and c[-4:] == ["--session-id", "S2", "--model", "haiku"]
    assert build_command("claude", "S", True, "accept-edits")[7:9] == ["--permission-mode", "acceptEdits"]
    with pytest.raises(ValueError):
        ClaudeSender("S", permissions="nope")


def test_user_line_is_one_stream_json_message():
    d = json.loads(user_line("héllo \"q\"\nline2"))
    assert d == {"type": "user", "message": {"role": "user", "content": [{"type": "text", "text": "héllo \"q\"\nline2"}]}}
    assert user_line("x").endswith(b"\n") and user_line("a\nb").count(b"\n") == 1


def test_clean_env_drops_variables_that_tie_the_child_to_the_parent_session():
    env = {"PATH": "/bin", "HOME": "/h", "ANTHROPIC_API_KEY": "k", "CLAUDE_CODE_SESSION_ID": "x",
           "CLAUDE_CODE_REMOTE": "true", "CLAUDE_CODE_REMOTE_SESSION_ID": "y", "CLAUDECODE": "1",
           "CLAUDE_CODE_CHILD_SESSION": "1", "SESSION_INGRESS_URL": "u", "CLAUDE_CODE_MESSAGING_TOKEN": "t",
           "CLAUDE_EFFORT": "high"}
    got = clean_env(env)
    assert got == {"PATH": "/bin", "HOME": "/h", "ANTHROPIC_API_KEY": "k", "CLAUDE_EFFORT": "high"}


def test_resume_send_busy_cycle_and_stdin_delivery(fake_env):
    async def run():
        s = ClaudeSender("sess-1", str(fake_env), resume=True, permissions="all", claude_bin=FAKE)
        assert s.available and not s.alive and not s.busy
        await s.send("hello there")
        assert s.busy and s.alive and s.sent == 1
        await until(lambda: not s.busy)
        assert s.last_result["subtype"] == "success" and s.last_error == ""
        t = find_transcript("sess-1", fake_env / "projects")
        assert t is not None
        lines = [json.loads(x) for x in t.read_text().splitlines()]
        assert [x["type"] for x in lines] == ["user", "assistant"]
        assert lines[0]["message"]["content"] == "hello there" and lines[1]["message"]["content"][0]["text"] == "echo: hello there"
        await s.send("second")                            # same process, no restart
        await until(lambda: not s.busy)
        assert len(calls(fake_env)) == 1
        c = calls(fake_env)[0]
        assert c["argv"][-2:] == ["--resume", "sess-1"] and "--dangerously-skip-permissions" in c["argv"]
        await s.stop()
        assert not s.alive
    asyncio.run(run())


def test_child_never_sees_the_parent_session_variables(fake_env, monkeypatch):
    monkeypatch.setenv("CLAUDE_CODE_SESSION_ID", "parent-session")

    async def run():
        s = ClaudeSender("sess-2", str(fake_env), claude_bin=FAKE)
        await s.send("hi")
        await until(lambda: not s.busy)
        await s.stop()
    asyncio.run(run())
    assert calls(fake_env)[0]["leaked"] == []


def test_new_session_uses_session_id_flag_and_cwd(fake_env):
    async def run():
        s = ClaudeSender(None, str(fake_env), resume=False, claude_bin=FAKE)
        sid = s.session_id
        await s.send("start")
        await until(lambda: not s.busy)
        await s.stop()
        return sid
    sid = asyncio.run(run())
    c = calls(fake_env)[0]
    assert c["argv"][-2:] == ["--session-id", sid] and Path(c["cwd"]).resolve() == fake_env.resolve()


def test_error_result_is_surfaced_and_process_restarts_after_exit(fake_env, monkeypatch):
    async def run():
        monkeypatch.setenv("FAKE_MODE", "fail")
        s = ClaudeSender("sess-3", str(fake_env), claude_bin=FAKE)
        await s.send("x")
        await until(lambda: not s.busy)
        assert s.last_error == "model overloaded"
        await s.stop()
        monkeypatch.setenv("FAKE_MODE", "exit")             # answers once, then the process ends
        s2 = ClaudeSender("sess-4", str(fake_env), claude_bin=FAKE)
        await s2.send("one")
        await until(lambda: not s2.alive and not s2.busy)
        await s2.send("two")                                # restarted, and resumes the session
        await until(lambda: not s2.alive and not s2.busy)
        assert s2.sent == 2
        await s2.stop()
    asyncio.run(run())
    argvs = [c["argv"] for c in calls(fake_env)]
    assert argvs[-1][-2:] == ["--resume", "sess-4"] and len([a for a in argvs if "sess-4" in a]) == 2


def test_missing_binary_is_a_clear_error_not_a_crash(tmp_path):
    async def run():
        s = ClaudeSender("x", str(tmp_path), claude_bin="definitely-not-claude")
        assert not s.available
        with pytest.raises(FileNotFoundError):
            await s.send("hi")
        assert "not found" in s.last_error and not s.busy
    asyncio.run(run())


def test_interrupt_stops_a_running_turn(fake_env, monkeypatch):
    monkeypatch.setenv("FAKE_DELAY", "30")

    async def run():
        s = ClaudeSender("sess-5", str(fake_env), claude_bin=FAKE)
        await s.send("long task")
        await until(lambda: (fake_env / "argv.jsonl").exists())
        assert s.busy
        await s.interrupt()
        assert not s.alive and not s.busy
    asyncio.run(run())
