"""Pure state reducer: Events in, agent tree + stats out. No UI, no I/O."""
from __future__ import annotations

import os
from dataclasses import dataclass, field

from . import pricing
from .model import (ADVISOR, AGENT_END, AGENT_START, ALIAS, JEV, LOG, MAIN, TURN,
                    AdvisorStats, AgentNode, Event, JevStats)


@dataclass
class Summary:
    fresh: int = 0
    write: int = 0
    read: int = 0
    out: int = 0
    turns: int = 0
    unpriced_turns: int = 0
    cost: pricing.Cost = field(default_factory=pricing.Cost)

    @property
    def prompt(self) -> int:
        return self.fresh + self.write + self.read

    @property
    def hit_rate(self) -> float | None:
        """Share of prompt tokens served from cache (None before any prompt)."""
        return self.read / self.prompt if self.prompt else None


def hit_rate(fresh: int, write: int, read: int) -> float | None:
    total = fresh + write + read
    return read / total if total else None


# A subagent that announced itself but shows no event for this long is not shown as running.
def _stale_secs() -> float:
    try:
        return float(os.environ.get("AGENTS_TREE_STALE", 120))
    except ValueError:
        return 120.0


STALE_SECS = _stale_secs()


class Store:
    def __init__(self) -> None:
        self.clock = 0.0  # newest event timestamp seen (virtual "now" for replays)
        self.nodes: dict[str, AgentNode] = {}
        self.advisor = AdvisorStats()
        self.jev = JevStats()
        self.log: list[tuple[float, str]] = []
        self._alias: dict[str, str] = {}
        self._pending_alias: dict[str, str] = {}  # alias seen before its node exists
        self.nodes[MAIN] = AgentNode(MAIN, "main")

    # -- lookup ---------------------------------------------------------
    def resolve(self, agent_id: str) -> str:
        return self._alias.get(agent_id, agent_id)

    def get(self, agent_id: str) -> AgentNode | None:
        return self.nodes.get(self.resolve(agent_id))

    def _add_alias(self, node: AgentNode, alias: str) -> None:
        node.aliases.add(alias)
        self._alias[alias] = node.id

    # -- reducer --------------------------------------------------------
    def apply(self, ev: Event) -> None:
        if ev.ts:
            self.clock = max(self.clock, ev.ts)
            node = self.get(ev.agent_id)
            if node and ev.kind in (AGENT_START, TURN, ALIAS):
                node.last_active = max(node.last_active, ev.ts)
        handler = {
            AGENT_START: self._start, AGENT_END: self._end, ALIAS: self._on_alias,
            TURN: self._turn, ADVISOR: self._advisor, JEV: self._jev, LOG: self._log,
        }.get(ev.kind)
        if handler:
            handler(ev)

    def _start(self, ev: Event) -> None:
        d = ev.data
        existing = self.get(ev.agent_id)
        if existing is None and d.get("match_kind"):
            # A hook announced an agent the transcript may already know by its tool_use id.
            existing = next((n for n in self.nodes.values()
                             if n.kind == d["match_kind"] and n.status == "running"
                             and n.id != MAIN and not n.aliases), None)
            if existing:
                self._add_alias(existing, ev.agent_id)
        if existing:
            existing.model = d.get("model") or existing.model
            existing.desc = d.get("desc") or existing.desc
            return
        parent = self.resolve(ev.parent_id) if ev.parent_id else MAIN
        if parent not in self.nodes:
            parent = MAIN
        node = AgentNode(ev.agent_id, d.get("kind", "agent"), parent,
                         d.get("model", ""), d.get("effort", ""), "running",
                         d.get("desc", ""), started=ev.ts)
        self.nodes[node.id] = node
        self.nodes[parent].children.append(node.id)
        if node.id in self._pending_alias:
            self._add_alias(node, self._pending_alias.pop(node.id))
        self._log(Event(ev.ts, LOG, data={"text": f"+ {node.kind} started"
                                          + (f" · {node.desc}" if node.desc else "")}))

    def _end(self, ev: Event) -> None:
        node = self.get(ev.agent_id)
        if node is None or node.id == MAIN or node.status != "running":
            return
        node.status = ev.data.get("status", "done")
        node.ended = ev.ts
        self._log(Event(ev.ts, LOG, data={"text": f"- {node.kind} {node.status}"}))

    def _on_alias(self, ev: Event) -> None:
        node = self.get(ev.agent_id)
        alias = ev.data.get("alias")
        if node and alias:
            self._add_alias(node, alias)
        elif alias:
            self._pending_alias[ev.agent_id] = alias

    def _claim(self, agent_id: str) -> AgentNode | None:
        """Bind an unknown sidechain id to the oldest running subagent without an alias.

        Subagent transcripts can be read before the tool_result that links their
        agent id to the spawning tool_use, so this is a best-effort heuristic.
        """
        node = next((n for n in self.nodes.values()
                     if n.id != MAIN and n.status == "running" and not n.aliases), None)
        if node:
            self._add_alias(node, agent_id)
        return node

    def _turn(self, ev: Event) -> None:
        node = self.get(ev.agent_id)
        if node is None and ev.data.get("sidechain"):
            node = self._claim(ev.agent_id)
        if node is None:
            return
        d = ev.data
        u = d.get("usage") or {}
        fresh = u.get("input_tokens", 0)
        c_write = u.get("cache_creation_input_tokens", 0)
        c_read = u.get("cache_read_input_tokens", 0)
        t_out = u.get("output_tokens", 0)
        # The 1h/5m split is optional in usage; unsplit cache writes count as 5-minute.
        w1h = min((u.get("cache_creation") or {}).get("ephemeral_1h_input_tokens", 0), c_write)
        w5m = c_write - w1h
        node.turns += 1
        node.fresh += fresh
        node.cache_write += c_write
        node.cache_read += c_read
        node.tokens_in += fresh + c_write + c_read
        node.tokens_out += t_out
        node.activity = d.get("tool", "")
        node.model = d.get("model") or node.model
        node.effort = d.get("effort") or node.effort
        cost = pricing.turn_cost(d.get("model") or node.model, fresh, w5m, w1h, c_read, t_out)
        if cost is None:
            node.unpriced_turns += 1
        else:
            node.cost_parts = node.cost_parts or pricing.Cost()
            node.cost_parts.add(cost)
            node.cost = node.cost_parts.total
        if d.get("advisor_model"):
            self.advisor.model = d["advisor_model"]

    def _advisor(self, ev: Event) -> None:
        if ev.data.get("count", True):
            self.advisor.calls += 1
            self._log(Event(ev.ts, LOG, data={"text": "advisor called"}))
        self.advisor.model = ev.data.get("model") or self.advisor.model
        if ev.data.get("advice"):
            self.advisor.last_advice = ev.data["advice"]

    def _jev(self, ev: Event) -> None:
        d = ev.data
        name = d.get("decision", "?")
        self.jev.forks += 1
        row = self.jev.decisions.setdefault(name, [0, 0.0, 0])
        row[0] += 1
        row[1] += d.get("confidence") or 0.0
        row[2] += 1 if d.get("escalate") else 0

    def _log(self, ev: Event) -> None:
        self.log.append((ev.ts, ev.data.get("text", "")))
        del self.log[:-500]

    # -- derived --------------------------------------------------------
    def state(self, node: AgentNode, now: float | None = None) -> str:
        """running | done | failed | stale. Only truly active agents read as running.

        `now` is wall-clock time when following a live session; None uses the newest
        event time, which is what a replay needs.
        """
        if node.status != "running":
            return node.status
        ref = self.clock if now is None else now
        last = node.last_active or node.started
        return "stale" if last and ref - last > STALE_SECS else "running"

    def _alive(self, node_id: str, now: float | None) -> bool:
        node = self.nodes[node_id]
        return self.state(node, now) == "running" or any(self._alive(c, now) for c in node.children)

    def visible_children(self, node_id: str, now: float | None = None, show_all: bool = False) -> list[str]:
        """Children to draw: all of them, or only agents actually running (plus the
        ancestors needed to keep a running agent connected to the tree)."""
        kids = self.nodes[node_id].children
        return list(kids) if show_all else [c for c in kids if self._alive(c, now)]

    def summary(self) -> "Summary":
        """Session-wide token and cost totals, split by category."""
        s = Summary()
        for n in self.nodes.values():
            s.fresh += n.fresh
            s.write += n.cache_write
            s.read += n.cache_read
            s.out += n.tokens_out
            s.turns += n.turns
            s.unpriced_turns += n.unpriced_turns
            if n.cost_parts:
                s.cost.add(n.cost_parts)
        return s

    def running_subagents(self, now: float | None = None) -> tuple[int, int]:
        subs = [n for n in self.nodes.values() if n.id != MAIN]
        return sum(1 for n in subs if self.state(n, now) == "running"), len(subs)
