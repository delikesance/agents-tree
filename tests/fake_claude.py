#!/usr/bin/env python3
"""Stand-in for `claude -p --input-format stream-json`: no network, no cost.

Records its argv/env to $FAKE_ARGV, and for every JSON user message on stdin appends a user line and an
assistant reply to the session transcript under $FAKE_PROJECTS, then prints a stream-json `result`.
FAKE_MODE=fail makes the result an error; FAKE_MODE=exit makes the process exit after the first message.
"""
import json
import os
import sys
import time
import uuid
from datetime import datetime, timezone

argv = sys.argv[1:]
sid = argv[argv.index("--session-id") + 1] if "--session-id" in argv else argv[argv.index("--resume") + 1]
with open(os.environ["FAKE_ARGV"], "a") as f:
    f.write(json.dumps({"argv": argv, "cwd": os.getcwd(),
                        "leaked": [k for k in os.environ if k.startswith("CLAUDE_CODE_SESSION")]}) + "\n")
proj = os.path.join(os.environ["FAKE_PROJECTS"], "-fake-proj")
os.makedirs(proj, exist_ok=True)
path = os.path.join(proj, sid + ".jsonl")


def now():
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def append(d):
    with open(path, "a") as f:
        f.write(json.dumps(d) + "\n")


print(json.dumps({"type": "system", "subtype": "init", "session_id": sid}), flush=True)
for raw in sys.stdin:
    text = json.loads(raw)["message"]["content"][0]["text"]
    append({"type": "user", "uuid": str(uuid.uuid4()), "timestamp": now(), "cwd": os.getcwd(),
            "origin": {"kind": "human"}, "turnOrigin": "human", "message": {"role": "user", "content": text}})
    time.sleep(float(os.environ.get("FAKE_DELAY", "0.3")))
    if os.environ.get("FAKE_MODE") == "fail":
        print(json.dumps({"type": "result", "subtype": "error", "is_error": True, "result": "model overloaded"}), flush=True)
        continue
    append({"type": "assistant", "uuid": str(uuid.uuid4()), "timestamp": now(), "message": {
        "model": "claude-sonnet-5-5", "usage": {"output_tokens": 3},
        "content": [{"type": "text", "text": "echo: " + text}]}})
    print(json.dumps({"type": "result", "subtype": "success", "is_error": False, "session_id": sid}), flush=True)
    if os.environ.get("FAKE_MODE") == "exit":
        break
