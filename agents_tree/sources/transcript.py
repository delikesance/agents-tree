"""Turn Claude Code transcript JSONL lines into normalized Events."""
from __future__ import annotations

import json
import os
from datetime import datetime
from pathlib import Path
from typing import Iterator

import re

from ..model import (ADVISOR, AGENT_END, AGENT_START, ALIAS, JEV, MAIN, MESSAGE, MSG_UPDATE,
                     TURN, Event, Message)

SPAWN_TOOLS = {"Agent", "Task"}
HIDDEN_TOOLS = {"SubagentHandback"}      # the hand-back is shown as the delegation's report
MAX_TEXT = 6000
JEV_PREFIX = "mcp__jev__"


def _ts(value: str | None) -> float:
    if not value:
        return 0.0
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp()
    except ValueError:
        return 0.0


def _text(content) -> str:
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "\n".join(c.get("text", "") for c in content if isinstance(c, dict))
    return ""


_REMINDER = re.compile(r"<system-reminder>.*?</system-reminder>", re.S)
_TAG = re.compile(r"</?[a-zA-Z][\w-]*(?:\s[^>]*)?>")


def clean_text(text: str) -> str:
    """Strip injected reminders and markup tags from a prompt, keeping the human text."""
    text = _REMINDER.sub("", text)
    cmd = re.search(r"<command-name>\s*(/?[^<\s]+)\s*</command-name>", text)
    if cmd:
        args = re.search(r"<command-args>(.*?)</command-args>", text, re.S)
        return (cmd.group(1) + (" " + args.group(1).strip() if args and args.group(1).strip() else "")).strip()
    return _TAG.sub("", text).strip()


def tool_detail(name: str, inp: dict) -> str:
    """One short line describing what a tool call does."""
    def first_line(s: str) -> str:
        return (s or "").strip().splitlines()[0][:120] if (s or "").strip() else ""
    if name == "Bash":
        cmd = (inp.get("command") or "").strip().splitlines()
        if len(cmd) > 1 and "<<" in cmd[0]:          # heredoc: the first line alone says nothing
            return f"{cmd[0][:40]} ⏎ {cmd[1].strip()}"[:120]
        return first_line(inp.get("command", ""))
    if name in ("Read", "Edit", "Write", "NotebookEdit", "MultiEdit"):
        parts = (inp.get("file_path") or inp.get("notebook_path") or "").split("/")
        return "/".join(parts[-3:])
    if name in ("Grep", "Glob"):
        return first_line(inp.get("pattern", "")) + (f"  in {inp['path']}" if inp.get("path") else "")
    if name in ("WebFetch", "WebSearch"):
        return first_line(inp.get("url") or inp.get("query") or "")
    if name in SPAWN_TOOLS:
        return first_line(inp.get("description", ""))
    for v in inp.values():            # MCP and other tools: first short string argument
        if isinstance(v, str) and v.strip():
            return first_line(v)
    return ""


class TranscriptParser:
    """Stateful per-file parser (needs to pair tool_use with its tool_result)."""

    def __init__(self, agent_id: str = MAIN, sidechain: bool = False) -> None:
        self.agent_id = agent_id
        self.sidechain = sidechain
        self._pending: dict[str, tuple[str, str]] = {}  # tool_use_id -> (kind, name)
        self._tool_ts: dict[str, float] = {}            # tool_use_id -> timestamp of the call
        self._seen_prompt = False                       # sidechain: first user turn = delegated prompt
        self._n = 0                                     # fallback id counter
        self._seen_msg: set[str] = set()                # API message ids whose usage was already counted

    def _id(self, d: dict, suffix: str = "") -> str:
        self._n += 1
        return f"{d.get('uuid') or f'{self.agent_id}-{self._n}'}{suffix}"

    def parse(self, d: dict) -> Iterator[Event]:
        t = d.get("type")
        ts = _ts(d.get("timestamp"))
        msg = d.get("message") or {}
        content = msg.get("content")
        if t == "assistant" and isinstance(content, list):
            yield from self._assistant(d, msg, content, ts)
        elif t == "user" and isinstance(content, (list, str)):
            yield from self._user(d, content, ts)

    def _assistant(self, d, msg, content, ts):
        usage = msg.get("usage")
        mid = msg.get("id")
        # Claude Code writes one transcript line per content block, each repeating the usage of the
        # whole API request: count that usage once per message id.
        dup = bool(mid) and mid in self._seen_msg
        if mid:
            self._seen_msg.add(mid)
        if usage:
            yield Event(ts, TURN, self.agent_id, data={"dup": dup,
                "model": msg.get("model", ""),
                "effort": d.get("perTurnEffort") or d.get("effort") or "",
                "usage": usage, "sidechain": self.sidechain,
                "advisor_model": d.get("advisorModel"), "tool": _last_tool(content)})
        model = msg.get("model", "")
        for i, c in enumerate(content):
            if c.get("type") == "text" and (c.get("text") or "").strip():
                yield Event(ts, MESSAGE, self.agent_id, data={"message": Message(
                    self._id(d, f":{i}"), ts, self.agent_id, "assistant",
                    c["text"].strip()[:MAX_TEXT], model=model)})
            if c.get("type") not in ("tool_use", "server_tool_use"):
                continue
            name, tid, inp = c.get("name", ""), c.get("id", ""), c.get("input") or {}
            self._tool_ts[tid] = ts
            if name in SPAWN_TOOLS:
                kind = inp.get("subagent_type") or "agent"
                self._pending[tid] = ("spawn", name)
                yield Event(ts, AGENT_START, tid, self.agent_id, {
                    "kind": kind, "desc": inp.get("description", ""), "model": inp.get("model", "")})
                yield Event(ts, MESSAGE, self.agent_id, data={"message": Message(
                    "deleg:" + tid, ts, self.agent_id, "delegation",
                    (inp.get("prompt") or "").strip()[:MAX_TEXT], kind=kind,
                    desc=inp.get("description", ""), model=inp.get("model", ""), target=tid)})
            elif name not in HIDDEN_TOOLS:
                yield Event(ts, MESSAGE, self.agent_id, data={"message": Message(
                    tid, ts, self.agent_id, "tool", tool=name.split("__")[-1],
                    detail=tool_detail(name, inp), status="running")})
            if name in SPAWN_TOOLS:
                pass
            elif name == "advisor":
                self._pending[tid] = ("advisor", name)
                yield Event(ts, ADVISOR, self.agent_id, data={"model": d.get("advisorModel", "")})
            elif name.startswith(JEV_PREFIX):
                self._pending[tid] = ("jev", name[len(JEV_PREFIX):])

    def _user(self, d, content, ts):
        blocks = [{"type": "text", "text": content}] if isinstance(content, str) else content
        if any(isinstance(c, dict) and c.get("type") == "tool_result" for c in blocks):
            for c in blocks:
                if isinstance(c, dict) and c.get("type") == "tool_result":
                    yield from self._tool_result(d, c, ts)
            return
        yield from self._user_message(d, blocks, ts)

    def _user_message(self, d, blocks, ts):
        if self.sidechain and not self._seen_prompt:
            self._seen_prompt = True            # the delegated prompt is already shown as the delegation
            return
        text = "\n".join(c.get("text", "") for c in blocks if isinstance(c, dict) and c.get("type") == "text")
        images = sum(1 for c in blocks if isinstance(c, dict) and c.get("type") == "image")
        cleaned = clean_text(text)
        if images:
            cleaned = (cleaned + "  " if cleaned else "") + "▣ " + ("image" if images == 1 else f"{images} images")
        if not cleaned:
            return
        origin = d.get("origin") or {}
        human = (origin.get("kind") == "human" or d.get("turnOrigin") == "human"
                 or (not origin and not d.get("isMeta")))
        role = "user" if human and not d.get("isMeta") else "system"
        if cleaned.startswith("[Request interrupted"):
            role = "system"
        yield Event(ts, MESSAGE, self.agent_id, data={"message": Message(
            self._id(d), ts, self.agent_id, role, cleaned[:MAX_TEXT])})

    def _tool_result(self, d, c, ts):
        tid = c.get("tool_use_id", "")
        started = self._tool_ts.pop(tid, None)
        duration = round(ts - started, 2) if started and ts >= started else None
        failed = bool(c.get("is_error"))
        kind_name = self._pending.pop(tid, None)
        if kind_name is None or kind_name[0] != "spawn":
            yield Event(ts, MSG_UPDATE, self.agent_id, data={
                "id": tid, "status": "error" if failed else "ok", "duration": duration})
        if not kind_name:
            return
        kind, name = kind_name
        if kind == "spawn":
            res = d.get("toolUseResult")
            aid = res.get("agentId") if isinstance(res, dict) else None
            if aid:
                yield Event(ts, ALIAS, tid, data={"alias": aid})
            yield Event(ts, AGENT_END, tid, data={"status": "failed" if failed else "done"})
            yield Event(ts, MESSAGE, self.agent_id, data={"message": Message(
                "rep:" + tid, ts, self.agent_id, "report",
                _text(c.get("content")).split("agentId:")[0].strip()[:MAX_TEXT],
                status="failed" if failed else "done", duration=duration, target=tid)})
            yield Event(ts, MSG_UPDATE, self.agent_id, data={
                "id": "deleg:" + tid, "status": "failed" if failed else "done", "duration": duration})
        elif kind == "jev":
            parsed = _parse_json(_text(c.get("content")))
            yield Event(ts, JEV, self.agent_id, data={
                "decision": name,
                "confidence": _num(parsed.get("confidence")),
                "escalate": bool(parsed.get("escalate"))})
        elif kind == "advisor":
            yield Event(ts, ADVISOR, self.agent_id, data={
                "advice": _text(c.get("content")).strip()[:200], "count": False})


def _last_tool(content: list) -> str:
    """Name of the last tool the assistant called in this message ('' if none)."""
    names = [c.get("name", "") for c in content if isinstance(c, dict)
             and c.get("type") in ("tool_use", "server_tool_use")]
    return names[-1].split("__")[-1] if names else ""


def _parse_json(text: str) -> dict:
    try:
        v = json.loads(text)
        return v if isinstance(v, dict) else {}
    except ValueError:
        return {}


def _num(v) -> float | None:
    return float(v) if isinstance(v, (int, float)) else None


def read_events(path: str | os.PathLike, agent_id: str = MAIN, sidechain: bool = False) -> list[Event]:
    parser = TranscriptParser(agent_id, sidechain)
    events: list[Event] = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            try:
                events.extend(parser.parse(json.loads(line)))
            except ValueError:
                continue
    return events


def session_files(session_jsonl: str | os.PathLike) -> list[tuple[Path, str, bool]]:
    """Main transcript plus any subagent transcripts stored next to it."""
    p = Path(session_jsonl)
    files = [(p, MAIN, False)]
    sub = p.with_suffix("") / "subagents"
    if sub.is_dir():
        for f in sorted(sub.glob("agent-*.jsonl")):
            files.append((f, f.stem.removeprefix("agent-"), True))
    return files


def meta_alias(path: Path, ts: float = 0.0) -> Event | None:
    """Deterministic agent-id link from agent-<id>.meta.json ({"toolUseId": ...})."""
    try:
        tool_use_id = json.loads(path.with_suffix(".meta.json").read_text()).get("toolUseId")
    except (OSError, ValueError):
        return None
    aid = path.stem.removeprefix("agent-")
    return Event(ts, ALIAS, tool_use_id, data={"alias": aid}) if tool_use_id else None


def read_session(session_jsonl: str | os.PathLike) -> list[Event]:
    events: list[Event] = []
    for path, aid, side in session_files(session_jsonl):
        events.extend(read_events(path, aid, side))
        if side and (alias := meta_alias(path)):
            events.append(alias)
    events.sort(key=lambda e: e.ts)
    return events
