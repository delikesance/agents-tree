"""Turn Claude Code transcript JSONL lines into normalized Events."""
from __future__ import annotations

import json
import os
from datetime import datetime
from pathlib import Path
from typing import Iterator

from ..model import (ADVISOR, AGENT_END, AGENT_START, ALIAS, JEV, MAIN, TURN, Event)

SPAWN_TOOLS = {"Agent", "Task"}
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


class TranscriptParser:
    """Stateful per-file parser (needs to pair tool_use with its tool_result)."""

    def __init__(self, agent_id: str = MAIN, sidechain: bool = False) -> None:
        self.agent_id = agent_id
        self.sidechain = sidechain
        self._pending: dict[str, tuple[str, str]] = {}  # tool_use_id -> (kind, name)

    def parse(self, d: dict) -> Iterator[Event]:
        t = d.get("type")
        ts = _ts(d.get("timestamp"))
        msg = d.get("message") or {}
        content = msg.get("content")
        if t == "assistant" and isinstance(content, list):
            yield from self._assistant(d, msg, content, ts)
        elif t == "user" and isinstance(content, list):
            yield from self._user(d, content, ts)

    def _assistant(self, d, msg, content, ts):
        usage = msg.get("usage")
        if usage:
            yield Event(ts, TURN, self.agent_id, data={
                "model": msg.get("model", ""),
                "effort": d.get("perTurnEffort") or d.get("effort") or "",
                "usage": usage, "sidechain": self.sidechain,
                "advisor_model": d.get("advisorModel")})
        for c in content:
            if c.get("type") not in ("tool_use", "server_tool_use"):
                continue
            name, tid, inp = c.get("name", ""), c.get("id", ""), c.get("input") or {}
            if name in SPAWN_TOOLS:
                self._pending[tid] = ("spawn", name)
                yield Event(ts, AGENT_START, tid, self.agent_id, {
                    "kind": inp.get("subagent_type") or "agent",
                    "desc": inp.get("description", ""), "model": inp.get("model", "")})
            elif name == "advisor":
                self._pending[tid] = ("advisor", name)
                yield Event(ts, ADVISOR, self.agent_id, data={"model": d.get("advisorModel", "")})
            elif name.startswith(JEV_PREFIX):
                self._pending[tid] = ("jev", name[len(JEV_PREFIX):])

    def _user(self, d, content, ts):
        for c in content:
            if c.get("type") != "tool_result":
                continue
            kind_name = self._pending.pop(c.get("tool_use_id", ""), None)
            if not kind_name:
                continue
            kind, name = kind_name
            tid = c["tool_use_id"]
            if kind == "spawn":
                res = d.get("toolUseResult")
                aid = res.get("agentId") if isinstance(res, dict) else None
                if aid:
                    yield Event(ts, ALIAS, tid, data={"alias": aid})
                yield Event(ts, AGENT_END, tid, data={
                    "status": "failed" if c.get("is_error") else "done"})
            elif kind == "jev":
                parsed = _parse_json(_text(c.get("content")))
                yield Event(ts, JEV, self.agent_id, data={
                    "decision": name,
                    "confidence": _num(parsed.get("confidence")),
                    "escalate": bool(parsed.get("escalate"))})
            elif kind == "advisor":
                yield Event(ts, ADVISOR, self.agent_id, data={
                    "advice": _text(c.get("content")).strip()[:200], "count": False})


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
