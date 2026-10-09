from __future__ import annotations

import argparse
import os
import sys

from .sources.live import latest_session, list_sessions
from .sources.replay import Replay
from .sources.transcript import read_session
from .ui.app import AgentsTreeApp, make_live


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(prog="agents-tree", description="Visualise Claude Code agents as a tree")
    sub = ap.add_subparsers(dest="cmd", required=True)
    live = sub.add_parser("live", help="follow a running session")
    live.add_argument("session", nargs="?", help="session .jsonl (default: latest of this project; "
                      "use --pick to choose, or press s in the app)")
    live.add_argument("--pick", action="store_true", help="open the session picker on start")
    live.add_argument("--no-hooks", action="store_true", help="ignore the hook event file")
    sub.add_parser("sessions", help="list available sessions")
    rep = sub.add_parser("replay", help="replay a recorded session")
    rep.add_argument("session", help="session .jsonl")
    rep.add_argument("--speed", type=float, default=4.0)
    args = ap.parse_args(argv)

    if args.cmd == "sessions":
        from .ui.picker import ago, project_label
        for s in list_sessions(limit=50):
            print(f"{ago(s.mtime):>9}  {project_label(s.project, s.cwd):<24} {s.id[:8]}  {s.title}")
        return 0
    if args.cmd == "live":
        path = None if args.pick else (args.session or latest_session(cwd=os.getcwd()))
        if not path and not args.pick:
            print("no session transcript found; pass a .jsonl path or use --pick", file=sys.stderr)
            return 1
        if not path:
            make_live(None, hooks=not args.no_hooks).run()
            return 0
        if not os.path.isfile(path):
            print(f"session transcript not found: {path}", file=sys.stderr)
            return 1
        make_live(path, hooks=not args.no_hooks).run()
    else:
        if not os.path.isfile(args.session):
            print(f"session transcript not found: {args.session}", file=sys.stderr)
            return 1
        AgentsTreeApp(replay=Replay(read_session(args.session), speed=args.speed), session=args.session).run()
    return 0


if __name__ == "__main__":
    sys.exit(main())
