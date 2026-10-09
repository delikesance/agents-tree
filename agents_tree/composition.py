"""Where do the input tokens of a session come from?

Every API request re-sends the whole conversation, so a piece of content costs (roughly) its size times
the number of requests that come after it. This module attributes that volume to categories: the
context that was already there on the first request (system prompt, tools, skills, CLAUDE.md), what the
assistant wrote into tool calls (code, commands), what tools returned (per tool), and plain text. What
cannot be attributed (thinking, images, attachments, estimation error) stays in `other`.

Sizes are estimated as characters / 4; money is the session's input cost split in proportion to volume.
Context compaction shrinks the real context and is not modelled, so long compacted sessions overstate volume.
"""
from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from pathlib import Path

from . import pricing
from .sources.transcript import session_files


@dataclass
class ToolVol:
    n: int = 0
    tokens: float = 0.0       # estimated tokens produced once
    volume: float = 0.0       # tokens x later requests = input tokens they cause


@dataclass
class Composition:
    turns: int = 0                      # API requests
    prompt_tokens: float = 0.0          # sum of every request's input (fresh + cache write + cache read)
    input_cost: float = 0.0             # USD for that input, estimated (None-priced requests excluded)
    baseline: float = 0.0               # volume of the first request's context
    baseline_tokens: float = 0.0        # its size
    user_text: float = 0.0
    assistant_text: float = 0.0
    outputs: dict[str, ToolVol] = field(default_factory=dict)
    inputs: dict[str, ToolVol] = field(default_factory=dict)

    # -- derived ---------------------------------------------------------
    @property
    def outputs_volume(self) -> float:
        return sum(v.volume for v in self.outputs.values())

    @property
    def inputs_volume(self) -> float:
        return sum(v.volume for v in self.inputs.values())

    @property
    def other(self) -> float:
        known = self.baseline + self.user_text + self.assistant_text + self.outputs_volume + self.inputs_volume
        return max(0.0, self.prompt_tokens - known)

    def share(self, volume: float) -> float:
        return volume / self.prompt_tokens if self.prompt_tokens else 0.0

    def usd(self, volume: float) -> float:
        return self.share(volume) * self.input_cost

    def categories(self) -> list[tuple[str, float]]:
        """(label, volume) rows, largest first."""
        rows = [("context before the first message", self.baseline),
                ("code and commands written (tool calls)", self.inputs_volume),
                ("tool outputs", self.outputs_volume),
                ("your messages", self.user_text),
                ("assistant replies", self.assistant_text),
                ("other (thinking, images, files…)", self.other)]
        return sorted(rows, key=lambda r: -r[1])

    def what_if_compress_outputs(self, ratio: float = 0.7) -> tuple[float, float]:
        """(share of input tokens saved, USD) if tool outputs shrank by `ratio`."""
        saved = self.outputs_volume * ratio
        return self.share(saved), self.usd(saved)

    def merge(self, o: "Composition") -> None:
        self.turns += o.turns
        for f in ("prompt_tokens", "input_cost", "baseline", "baseline_tokens", "user_text", "assistant_text"):
            setattr(self, f, getattr(self, f) + getattr(o, f))
        for mine, theirs in ((self.outputs, o.outputs), (self.inputs, o.inputs)):
            for k, v in theirs.items():
                t = mine.setdefault(k, ToolVol())
                t.n += v.n
                t.tokens += v.tokens
                t.volume += v.volume


def _text_len(content) -> int:
    if isinstance(content, str):
        return len(content)
    if isinstance(content, list):
        return sum(len(b.get("text", "")) for b in content if isinstance(b, dict) and b.get("type") == "text")
    return 0


def _analyze_file(path: Path) -> Composition:
    """One transcript = one conversation context (the main session, or a single subagent)."""
    rows: list[dict] = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            try:
                rows.append(json.loads(line))
            except ValueError:
                continue

    # Pass 1: unique API requests, in order, with their input size.
    seen: dict[str, int] = {}
    prompts: list[float] = []
    cost = 0.0
    for d in rows:
        m = d.get("message") or {}
        u = m.get("usage")
        if d.get("type") != "assistant" or not u:
            continue
        key = m.get("id") or d.get("uuid")
        if key in seen:
            continue
        seen[key] = len(prompts) + 1
        fresh, write = u.get("input_tokens", 0), u.get("cache_creation_input_tokens", 0)
        read = u.get("cache_read_input_tokens", 0)
        w1h = min((u.get("cache_creation") or {}).get("ephemeral_1h_input_tokens", 0), write)
        prompts.append(fresh + write + read)
        c = pricing.turn_cost(m.get("model", ""), fresh, write - w1h, w1h, read, 0)
        cost += (c.fresh + c.write + c.read) if c else 0.0

    comp = Composition(turns=len(prompts), prompt_tokens=sum(prompts), input_cost=cost)
    if not prompts:
        return comp
    n = len(prompts)
    comp.baseline_tokens = prompts[0]
    comp.baseline = prompts[0] * n

    # Pass 2: attribute content to the requests that re-send it.
    tool_name: dict[str, str] = {}
    t = 0                                             # requests seen so far
    for d in rows:
        m = d.get("message") or {}
        content = m.get("content")
        if d.get("type") == "assistant":
            key = m.get("id") or d.get("uuid")
            if m.get("usage") and seen.get(key) == t + 1:
                t += 1
            later = n - t
            for b in content if isinstance(content, list) else []:
                if b.get("type") == "text":
                    comp.assistant_text += len(b.get("text", "")) / 4 * later
                elif b.get("type") in ("tool_use", "server_tool_use"):
                    tool_name[b.get("id", "")] = b.get("name", "?")
                    tokens = len(json.dumps(b.get("input") or {}, ensure_ascii=False)) / 4
                    v = comp.inputs.setdefault(b.get("name", "?").split("__")[-1], ToolVol())
                    v.n += 1
                    v.tokens += tokens
                    v.volume += tokens * later
        elif d.get("type") == "user":
            later = n - t
            blocks = [{"type": "text", "text": content}] if isinstance(content, str) else (content or [])
            for b in blocks:
                if not isinstance(b, dict):
                    continue
                if b.get("type") == "text":
                    comp.user_text += len(b.get("text", "")) / 4 * later
                elif b.get("type") == "tool_result":
                    tokens = _text_len(b.get("content")) / 4
                    name = tool_name.get(b.get("tool_use_id", ""), "?").split("__")[-1]
                    v = comp.outputs.setdefault(name, ToolVol())
                    v.n += 1
                    v.tokens += tokens
                    v.volume += tokens * later
    return comp


def analyze(session_jsonl: str | os.PathLike, subagents: bool = True) -> Composition:
    """Composition of a whole session: the main conversation plus each subagent's own."""
    total = Composition()
    for path, _aid, side in session_files(session_jsonl):
        if side and not subagents:
            continue
        if path.exists():
            total.merge(_analyze_file(path))
    return total
