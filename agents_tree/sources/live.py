"""Incremental readers: tail transcripts and the hook event file."""
from __future__ import annotations

import json
import os
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

    def __init__(self, path: str | os.PathLike | None = None) -> None:
        self._tail = _Tail(Path(path or os.environ.get("AGENTS_TREE_EVENTS", DEFAULT_EVENTS)))

    def poll(self) -> list[Event]:
        out: list[Event] = []
        for line in self._tail.lines():
            try:
                d = json.loads(line)
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
