from __future__ import annotations

import argparse
import os
import sys

from .sources.live import latest_session, list_sessions
from .sources.replay import Replay
from .sources.transcript import read_session
from .ui.app import AgentsTreeApp, make_live


def send_opts(args) -> dict:
    return {"permissions": args.permissions, "claude_bin": args.claude_bin, "can_send": not args.no_send}


def run_context(args) -> int:
    from . import composition
    from .ui.picker import project_label
    if args.recent:
        print(f"{'session':<10}{'project':<20}{'requests':>9}{'input $':>9}{'context':>9}{'tool in':>9}{'tool out':>9}{'other':>8}")
        for s in list_sessions(limit=args.recent):
            c = composition.analyze(s.path)
            if not c.turns:
                continue
            print(f"{s.id[:8]:<10}{project_label(s.project, s.cwd)[:19]:<20}{c.turns:>9}{c.input_cost:>9.2f}"
                  f"{c.share(c.baseline):>9.0%}{c.share(c.inputs_volume):>9.0%}{c.share(c.outputs_volume):>9.0%}"
                  f"{c.share(c.other):>8.0%}")
        return 0
    path = args.session or latest_session(cwd=os.getcwd())
    if not path or not os.path.isfile(path):
        print("session transcript not found; pass a .jsonl path", file=sys.stderr)
        return 1
    c = composition.analyze(path)
    if not c.turns:
        print("no API requests in this session yet")
        return 0
    print(f"{path}\n{c.turns} requests · {c.prompt_tokens / 1e6:.1f}M input tokens re-sent · input cost ~${c.input_cost:,.2f} (estimate)\n")
    for label, volume in c.categories():
        print(f"{c.share(volume):>6.1%}  ~${c.usd(volume):>7,.2f}  {label}")
    print("\ntool outputs by tool:")
    for name, v in sorted(c.outputs.items(), key=lambda x: -x[1].volume)[:8]:
        print(f"{c.share(v.volume):>6.1%}  ~${c.usd(v.volume):>7,.2f}  {name} ({v.n} calls, ~{int(v.tokens):,} tokens)")
    share, usd = c.what_if_compress_outputs()
    print(f"\nShrinking every tool output by 70% would remove about {share:.1%} of input tokens (~${usd:,.2f}).")
    return 0


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(prog="agents-tree", description="Visualise Claude Code agents as a tree")
    sub = ap.add_subparsers(dest="cmd", required=True)
    live = sub.add_parser("live", help="follow a running session")
    live.add_argument("session", nargs="?", help="session .jsonl (default: latest of this project; "
                      "use --pick to choose, or press s in the app)")
    live.add_argument("--pick", action="store_true", help="open the session picker on start")
    live.add_argument("--permissions", choices=["all", "accept-edits", "plan"], default="all",
                      help="what Claude may do when you message it from the TUI (default: all, no confirmations)")
    live.add_argument("--claude-bin", default="claude", help="claude executable used to send messages")
    live.add_argument("--no-send", action="store_true", help="read-only: hide the message box")
    live.add_argument("--no-hooks", action="store_true", help="ignore the hook event file")
    sub.add_parser("sessions", help="list available sessions")
    ctx = sub.add_parser("context", help="where a session's input tokens (and money) go")
    ctx.add_argument("session", nargs="?", help="session .jsonl (default: latest of this project)")
    ctx.add_argument("--recent", type=int, metavar="N", help="one summary line for each of the N latest sessions")
    rep = sub.add_parser("replay", help="replay a recorded session")
    rep.add_argument("session", help="session .jsonl")
    rep.add_argument("--speed", type=float, default=4.0)
    args = ap.parse_args(argv)

    if args.cmd == "sessions":
        from .ui.picker import ago, project_label
        for s in list_sessions(limit=50):
            print(f"{ago(s.mtime):>9}  {project_label(s.project, s.cwd):<24} {s.id[:8]}  {s.title}")
        return 0
    if args.cmd == "context":
        return run_context(args)
    if args.cmd == "live":
        path = None if args.pick else (args.session or latest_session(cwd=os.getcwd()))
        if not path and not args.pick:
            print("no session transcript found; pass a .jsonl path or use --pick", file=sys.stderr)
            return 1
        if not path:
            make_live(None, hooks=not args.no_hooks, **send_opts(args)).run()
            return 0
        if not os.path.isfile(path):
            print(f"session transcript not found: {path}", file=sys.stderr)
            return 1
        make_live(path, hooks=not args.no_hooks, **send_opts(args)).run()
    else:
        if not os.path.isfile(args.session):
            print(f"session transcript not found: {args.session}", file=sys.stderr)
            return 1
        AgentsTreeApp(replay=Replay(read_session(args.session), speed=args.speed), session=args.session).run()
    return 0


if __name__ == "__main__":
    sys.exit(main())
