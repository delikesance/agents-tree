"""Incremental readers: tail transcripts and the hook event file."""
from __future__ import annotations

import json
import os
from dataclasses import dataclass
from pathlib import Path

from ..model import AGENT_END, AGENT_START, MAIN, Event
from .transcript import TranscriptParser, meta_alias, session_files

DEFAULT_EVENTS = os.path.expanduser("~/.claude/agents-tree-events.jsonl")


class _Tail:
    def __init__(self, path: Path) -> None:
        self.path, self.offset, self.buf = path, 0, ""

    def lines(self) -> list[str]:
        try:
            with open(self.path, encoding="utf-8") as f:
                f.seek(self.offset)
                chunk = f.read()
                self.offset = f.tell()
        except OSError:
            return []
        self.buf += chunk
        *done, self.buf = self.buf.split("\n")
        return [x for x in done if x.strip()]


class TranscriptTailer:
    """Polls the main transcript and its subagent transcripts for new events."""

    def __init__(self, session_jsonl: str | os.PathLike) -> None:
        self.session = Path(session_jsonl)
        self._tails: dict[Path, tuple[_Tail, TranscriptParser]] = {}

    def poll(self) -> list[Event]:
        events: list[Event] = []
        for path, aid, side in session_files(self.session):
            if path not in self._tails:
                self._tails[path] = (_Tail(path), TranscriptParser(aid, side))
                if side and (alias := meta_alias(path)):
                    events.append(alias)
        for tail, parser in self._tails.values():
            for line in tail.lines():
                try:
                    events.extend(parser.parse(json.loads(line)))
                except ValueError:
                    continue
        events.sort(key=lambda e: e.ts)
        return events


class HookTailer:
    """Reads events appended by hooks/emit.py (already normalized)."""

    def __init__(self, path: str | os.PathLike | None = None, session_id: str | None = None) -> None:
        self._tail = _Tail(Path(path or os.environ.get("AGENTS_TREE_EVENTS", DEFAULT_EVENTS)))
        self.session_id = session_id  # keep only this session's events (events without one are kept)

    def poll(self) -> list[Event]:
        out: list[Event] = []
        for line in self._tail.lines():
            try:
                d = json.loads(line)
                if self.session_id and d.get("session") not in (None, self.session_id):
                    continue
                out.append(Event(d.get("ts", 0.0), d["kind"], d.get("agent_id", MAIN),
                                 d.get("parent_id"), d.get("data") or {}))
            except (ValueError, KeyError):
                continue
        return out


def latest_session(projects_dir: str | os.PathLike | None = None, cwd: str | None = None) -> Path | None:
    """Most recently modified session transcript, preferring the current project."""
    root = Path(projects_dir or os.path.expanduser("~/.claude/projects"))
    pattern = "*.jsonl"
    cands = []
    if cwd:
        d = root / cwd.replace("/", "-")
        if d.is_dir():
            cands = list(d.glob(pattern))
    if not cands:
        cands = list(root.glob(f"*/{pattern}"))
    return max(cands, key=lambda p: p.stat().st_mtime, default=None)


@dataclass
class SessionInfo:
    path: Path
    project: str      # project directory name as stored under ~/.claude/projects
    mtime: float
    title: str
    subagents: int
    cwd: str = ""

    @property
    def id(self) -> str:
        return self.path.stem


def _clean_prompt(text: str) -> str:
    """Drop injected tags/reminders so the first real user prompt can serve as a title."""
    import re
    text = re.sub(r"<system-reminder>.*?</system-reminder>", "", text, flags=re.S)
    text = re.sub(r"<[^>]+>", " ", text)
    return " ".join(text.split())


def scan_session(path: Path, max_bytes: int = 256_000, width: int = 80) -> tuple[str, str]:
    """(first user prompt, cwd) read from the head of a transcript."""
    try:
        with open(path, encoding="utf-8") as f:
            data = f.read(max_bytes)
    except OSError:
        return "", ""
    title, cwd = "", ""
    for line in data.splitlines():
        try:
            d = json.loads(line)
        except ValueError:
            continue
        cwd = cwd or d.get("cwd") or ""
        if title or d.get("type") != "user" or d.get("isSidechain"):
            if title and cwd:
                break
            continue
        content = (d.get("message") or {}).get("content")
        blocks = [content] if isinstance(content, str) else [
            b.get("text", "") for b in content or [] if isinstance(b, dict) and b.get("type") == "text"]
        for b in blocks:
            t = _clean_prompt(b)
            if t and not title:
                title = t if len(t) <= width else t[: width - 1] + "…"
    return title, cwd


def session_title(path: Path, **kw) -> str:
    return scan_session(path, **kw)[0]


def list_sessions(projects_dir: str | os.PathLike | None = None, limit: int | None = 200) -> list[SessionInfo]:
    """All session transcripts, most recently modified first."""
    root = Path(projects_dir or os.path.expanduser("~/.claude/projects"))
    files = []
    for p in root.glob("*/*.jsonl"):
        try:
            files.append((p.stat().st_mtime, p))
        except OSError:
            continue
    files.sort(reverse=True)
    out = []
    for mtime, p in files[:limit]:
        sub = p.with_suffix("") / "subagents"
        n = len(list(sub.glob("agent-*.jsonl"))) if sub.is_dir() else 0
        title, cwd = scan_session(p)
        out.append(SessionInfo(p, p.parent.name, mtime, title, n, cwd))
    return out
