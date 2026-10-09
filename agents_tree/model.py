from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

MAIN = "main"

# Event kinds produced by the sources and consumed by Store.apply.
AGENT_START = "agent_start"  # data: kind, model?, effort?, desc?
AGENT_END = "agent_end"      # data: status ("done"|"failed")
ALIAS = "alias"              # data: alias (another id naming the same agent)
TURN = "turn"                # data: model, effort, usage
ADVISOR = "advisor"          # data: model?, advice?
JEV = "jev"                  # data: decision, confidence?, escalate?
LOG = "log"                  # data: text
MESSAGE = "message"          # data: message (a Message to append to the chat)
MSG_UPDATE = "msg_update"    # data: id, plus Message fields to change (tool status, duration)


@dataclass
class Message:
    """One box in the chat. Roles: user | assistant | tool | delegation | report | system."""
    id: str
    ts: float
    agent_id: str                  # who produced it (resolve through Store.get for aliases)
    role: str
    text: str = ""
    tool: str = ""                 # tool name (role == tool)
    detail: str = ""               # short tool argument: command, path, pattern
    status: str = ""               # tool: running | ok | error; report: done | failed
    duration: float | None = None  # seconds, tool_use -> tool_result
    kind: str = ""                 # delegation/report: subagent type
    desc: str = ""                 # delegation: short description
    model: str = ""
    target: str = ""               # delegation/report: id of the delegated agent node
    rev: int = 0                   # bumped on every update so views know to re-render


@dataclass
class Event:
    ts: float
    kind: str
    agent_id: str = MAIN
    parent_id: str | None = None
    data: dict[str, Any] = field(default_factory=dict)


@dataclass
class AgentNode:
    id: str
    kind: str                      # "main", "worker", "explorer", ...
    parent: str | None = None
    model: str = ""
    effort: str = ""
    status: str = "running"        # running | done | failed
    desc: str = ""
    turns: int = 0
    tokens_in: int = 0             # all prompt tokens: fresh + cache writes + cache reads
    tokens_out: int = 0
    fresh: int = 0                 # uncached prompt tokens
    cache_write: int = 0
    cache_read: int = 0
    cost: float = 0.0              # estimated USD (known-priced turns only)
    cost_parts: object = None      # pricing.Cost split by category (set on first priced turn)
    unpriced_turns: int = 0        # turns whose model has no known price
    started: float = 0.0
    ended: float | None = None
    activity: str = ""            # tool the agent is using right now (from its last turn)
    last_active: float = 0.0       # timestamp of the last event seen for this agent
    children: list[str] = field(default_factory=list)
    aliases: set[str] = field(default_factory=set)


@dataclass
class AdvisorStats:
    model: str = ""
    calls: int = 0
    last_advice: str = ""


@dataclass
class JevStats:
    forks: int = 0
    # decision -> [count, confidence_sum, escalations]
    decisions: dict[str, list[float]] = field(default_factory=dict)

    def avg_confidence(self, decision: str) -> float | None:
        c = self.decisions.get(decision)
        if not c or not c[0]:
            return None
        return c[1] / c[0]
