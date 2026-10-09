#!/usr/bin/env python3
"""Claude Code hook: append a normalized event for agents-tree.

Reads the hook JSON payload on stdin and appends one line to
$AGENTS_TREE_EVENTS (default ~/.claude/agents-tree-events.jsonl).
Always exits 0 and never prints, so it cannot disturb the session.
"""
import json
import os
import sys
import time

PATH = os.environ.get("AGENTS_TREE_EVENTS", os.path.expanduser("~/.claude/agents-tree-events.jsonl"))


def normalize(p):
    name = p.get("hook_event_name", "")
    now = time.time()
    if name == "SubagentStart":
        kind = p.get("agent_type") or "agent"
        return {"ts": now, "kind": "agent_start", "agent_id": p.get("agent_id", ""),
                "data": {"kind": kind, "match_kind": kind}}
    if name == "SubagentStop":
        return {"ts": now, "kind": "agent_end", "agent_id": p.get("agent_id", ""),
                "data": {"status": "done"}}
    return None


def main():
    try:
        payload = json.load(sys.stdin)
        ev = normalize(payload)
        if ev and payload.get("session_id"):
            ev["session"] = payload["session_id"]
        if ev and ev["kind"] and (ev["agent_id"] if "agent_id" in ev else True):
            with open(PATH, "a", encoding="utf-8") as f:
                f.write(json.dumps(ev) + "\n")
    except Exception:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
