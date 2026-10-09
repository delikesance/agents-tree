"""Send messages to Claude Code from the TUI by owning a `claude` subprocess.

The subprocess runs `claude -p --input-format stream-json --output-format stream-json`, resumed
on the followed session (or a fresh `--session-id`). Messages are written to its stdin; what
Claude does lands in the normal transcript, so the chat and the tree pick it up through the usual
tailers. Stdout is only read to know when a turn ends (`result`) and to surface errors.

This cannot write into an interactive `claude` already open in another terminal: do not drive the
same session from both at once.
"""
from __future__ import annotations

import asyncio
import json
import os
import re
import shutil
import uuid
from pathlib import Path

# Permission presets -> CLI flags. `all` is the default the user chose: nothing can ask for
# confirmation in a headless run, so anything not allowed would simply be refused.
PERMISSIONS = {
    "all": ["--dangerously-skip-permissions"],
    "accept-edits": ["--permission-mode", "acceptEdits"],
    "plan": ["--permission-mode", "plan"],
}

# Variables that tie a Claude Code process to the session that launched agents-tree. Left in the
# child's environment they make it reuse that session's id and report into it.
_INHERITED = re.compile(r"^(CLAUDE_CODE_(SESSION|REMOTE|CHILD|MESSAGING|ENTRYPOINT|EXECPATH|DIAGNOSTICS)"
                        r"|CLAUDE_SESSION|SESSION_INGRESS|CLAUDECODE$|CLAUDE_PID$)")


def clean_env(env: dict[str, str] | None = None) -> dict[str, str]:
    return {k: v for k, v in (env if env is not None else os.environ).items() if not _INHERITED.match(k)}


def build_command(claude_bin: str, session_id: str, resume: bool, permissions: str = "all",
                  model: str | None = None) -> list[str]:
    cmd = [claude_bin, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
           *PERMISSIONS[permissions]]
    cmd += ["--resume", session_id] if resume else ["--session-id", session_id]
    if model:
        cmd += ["--model", model]
    return cmd


def user_line(text: str) -> bytes:
    msg = {"type": "user", "message": {"role": "user", "content": [{"type": "text", "text": text}]}}
    return (json.dumps(msg) + "\n").encode()


def find_transcript(session_id: str, projects_dir: str | os.PathLike | None = None) -> Path | None:
    root = Path(projects_dir or os.path.expanduser("~/.claude/projects"))
    return next(iter(root.glob(f"*/{session_id}.jsonl")), None)


class ClaudeSender:
    """One `claude` process bound to one session; restarted on demand after it exits."""

    def __init__(self, session_id: str | None = None, cwd: str | None = None, resume: bool = True,
                 permissions: str = "all", claude_bin: str = "claude", model: str | None = None,
                 env: dict[str, str] | None = None) -> None:
        if permissions not in PERMISSIONS:
            raise ValueError(f"permissions must be one of {sorted(PERMISSIONS)}")
        self.resume = resume and session_id is not None
        self.session_id = session_id or str(uuid.uuid4())
        self.cwd = cwd or os.getcwd()
        self.permissions, self.claude_bin, self.model = permissions, claude_bin, model
        self._env = clean_env(env)
        self.proc: asyncio.subprocess.Process | None = None
        self._reader: asyncio.Task | None = None
        self.busy = False            # a turn is in flight (between our send and its `result`)
        self.last_error = ""
        self.last_result: dict | None = None
        self.sent = 0
        self._stderr_tail: list[str] = []
        self._err_task: asyncio.Task | None = None

    @property
    def available(self) -> bool:
        return shutil.which(self.claude_bin) is not None or Path(self.claude_bin).is_file()

    @property
    def alive(self) -> bool:
        return self.proc is not None and self.proc.returncode is None

    async def start(self) -> None:
        if self.alive:
            return
        cmd = build_command(self.claude_bin, self.session_id, self.resume, self.permissions, self.model)
        self.proc = await asyncio.create_subprocess_exec(
            *cmd, cwd=self.cwd, env=self._env, stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
            limit=64 * 1024 * 1024)       # stream-json lines can be very large
        self._stderr_tail.clear()
        self._err_task = asyncio.create_task(self._drain_stderr(self.proc))
        self.resume = True            # once started the session exists: later restarts resume it
        self._reader = asyncio.create_task(self._read())

    async def send(self, text: str) -> None:
        """Write one user message; starts (or restarts) the process when needed."""
        text = text.strip()
        if not text:
            return
        if not self.available:
            self.last_error = f"`{self.claude_bin}` not found on PATH"
            raise FileNotFoundError(self.last_error)
        await self.start()
        assert self.proc and self.proc.stdin
        self.last_error = ""
        self.busy = True
        self.sent += 1
        self.proc.stdin.write(user_line(text))
        await self.proc.stdin.drain()

    async def _drain_stderr(self, proc) -> None:
        """Keep the pipe empty (a full pipe would block the child) and remember the last lines."""
        if proc.stderr is None:
            return
        async for raw in proc.stderr:
            self._stderr_tail = (self._stderr_tail + [raw.decode(errors="replace").rstrip()])[-20:]

    async def _read(self) -> None:
        proc = self.proc
        assert proc and proc.stdout
        async for raw in proc.stdout:
            try:
                d = json.loads(raw)
            except ValueError:
                continue
            if d.get("type") == "result":
                self.busy = False
                self.last_result = d
                if d.get("is_error"):
                    self.last_error = str(d.get("result") or d.get("subtype") or "error")[:200]
        # stdout closed: the process ended (crash, killed, or stdin closed)
        await proc.wait()
        if self._err_task:
            await asyncio.gather(self._err_task, return_exceptions=True)
        if self.busy:
            self.busy = False
            tail = [ln for ln in self._stderr_tail if ln.strip()]
            self.last_error = (tail[-1] if tail else f"claude exited with code {proc.returncode}")[:200]

    async def interrupt(self) -> None:
        """Stop the running turn (terminates the process; the next send resumes the session)."""
        if self.alive:
            self.proc.terminate()
            try:
                await asyncio.wait_for(self.proc.wait(), 5)
            except asyncio.TimeoutError:
                self.proc.kill()
        self.busy = False

    async def stop(self) -> None:
        if self.alive and self.proc.stdin:
            self.proc.stdin.close()
        await self.interrupt()
        for t in (self._reader, self._err_task):
            if t:
                t.cancel()
