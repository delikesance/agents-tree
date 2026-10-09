# agents-tree

Terminal chat for Claude Code, built to be easy to read. `agents-tree` starts a **new session**; you can open an existing one
at any time (`s`, or `agents-tree live <session.jsonl>`, or `-c` to continue the latest).

- one quiet column: your messages (blue bar), Claude's replies as plain text, tool calls collapsed into one line per run
  (`⚙ 6 tool calls · Bash ×5 · Write  ✓ 6`; failed and running ones stay visible), hook noise hidden;
- a strip above the message box listing the subagents working right now, only while there are some; on a wide terminal
  (150 columns or more) the strip becomes a **live side panel**: the agent diagram (who runs which tool, turns, cost,
  cache hit), the cost, and the latest activity, next to the conversation;
- delegations to subagents and their reports as small boxes, with the subagents' own activity indented under them;
- `d` opens the details: agents, cost with prompt-cache accounting, and "where tokens go" (which parts of the context are
  re-sent on every request); `t` shows the agent tree;
- the message box sends your text to Claude Code through a `claude -p` process (new or resumed session).

## Install

Go 1.26 or newer.

```
go install github.com/delikesance/agents-tree/cmd/agents-tree@latest
# or, from a checkout:
go build -o agents-tree ./cmd/agents-tree
```

## Makefile

`make` lists the targets: `make run` (build + new session), `make continue`, `make pick`, `make replay SESSION=file.jsonl`,
`make context`, `make baseline`, `make check` (format + vet + tests). Variables: `SESSION`, `PERMS=all|accept-edits|plan`, `SPEED`.

## Commands

```
agents-tree                         start a NEW session (same as: agents-tree live)
agents-tree live [session.jsonl]    open that session instead
      -c, --continue                continue the latest session of this directory
      --pick                        choose a session (or a new one) in the app
      --permissions all|accept-edits|plan
                                    what Claude may do when you message it (default: all)
      --claude-bin PATH             claude executable used to send messages
      --no-send                     read-only: no message box
      --no-hooks                    ignore the hook event file
agents-tree replay <session.jsonl> [--speed N]
agents-tree sessions                list sessions, newest first
agents-tree context [session.jsonl] [--recent N]
                                    where a session's input tokens and money go
agents-tree baseline [session.jsonl] [--recent N]
                                    what the first request contains and what could be cut (read-only)
agents-tree hook                    Claude Code hook: reads the payload on stdin, appends an event
agents-tree compress                Claude Code PostToolUse hook: shrinks long tool outputs
agents-tree hooks install|uninstall|status [--settings PATH] [--apply]
```

Transcripts are read from `~/.claude/projects/<project>/<session>.jsonl`, plus the subagent transcripts in `<session>/subagents/`.

## Keys

The message box has the focus from the start: type, `enter` sends, `alt+enter` adds a line. `esc` switches to command mode,
where letters are shortcuts (`i` or `enter` goes back to the box):

`s` open a session (or a new one) · `n` new session · `d` details (agents, cost, where tokens go; `h` inside: finished agents)
· `e` unfold long messages, tool calls and hidden system lines · `f` filter by agent · `t` chat / agent tree · `q` quit ·
`up` / `down` / `home` / `end` scroll.
Always available: `pgup` / `pgdown` scroll, `ctrl+k` stop Claude, `ctrl+c` stops Claude when it is working, otherwise quits.
Replay only: `space` pause, `+` / `-` speed.

## Permissions

Messages are sent with `claude -p --input-format stream-json ...` (`--session-id` for a new session, `--resume <session>` for an existing one). The default `--permissions all` passes
`--dangerously-skip-permissions`: in headless mode nothing can ask for confirmation, so Claude may edit files and run commands without asking
(the UI shows a warning). `--permissions accept-edits` or `plan` restrict it; anything not allowed is then refused, not asked.
Do not drive the same session from an interactive `claude` at the same time: agents-tree cannot write into a `claude` that is open elsewhere.

## Hooks (optional)

Hooks give SubagentStart / SubagentStop events without waiting for the transcript. Nothing installs them for you; add them to `~/.claude/settings.json` yourself:

```json
{
  "hooks": {
    "SubagentStart": [{ "hooks": [{ "type": "command", "command": "agents-tree hook" }] }],
    "SubagentStop":  [{ "hooks": [{ "type": "command", "command": "agents-tree hook" }] }]
  }
}
```

The command appends one JSON line per event to `$AGENTS_TREE_EVENTS`; `live` reads it for the followed session (`--no-hooks` to ignore). It never prints and always exits 0.

## Compressing tool outputs (optional)

`agents-tree hooks install` previews (dry run) the hooks it would add to `~/.claude/settings.json`; with `--apply` it
backs the file up (`.bak-<timestamp>`) and writes them. It manages two things, and only its own entries:

- `PostToolUse` on `Read|Grep|Glob|WebFetch|WebSearch|mcp__.*` → `agents-tree compress`: strips ANSI codes and
  repeated lines, and keeps the head and tail of outputs over 6000 bytes (the full text is saved under
  `~/.claude/agents-tree/tool-outputs/`; errors keep more; `Read` is never truncated). Sizes (never content) are logged to
  `~/.claude/agents-tree/compress-log.jsonl`. Hooks also run inside subagents.
- `SubagentStart` / `SubagentStop` → `agents-tree hook`.

For Bash use [RTK](https://github.com/rtk-ai/rtk) (`rtk init -g --auto-patch`); `hooks status` tells you whether it is there.
Measure first: on one real session tool outputs were ~7% of re-sent input tokens (`agents-tree context`), the context before
the first message ~22-29% (`agents-tree baseline`). Whether Claude Code accepts `updatedToolOutput` for every tool's output
shape was not verified; if it does not, the original output is used.

## Environment

- `AGENTS_TREE_STALE`: seconds without an event after which a subagent no longer counts as running (default 120).
- `AGENTS_TREE_PRICING`: JSON file with extra or overriding rates, e.g. `{"claude-opus-5-5": {"in": 4, "out": 20, "cache_read": 0.2}}` ($ per million tokens).
- `AGENTS_TREE_EVENTS`: hook event file (default `~/.claude/agents-tree-events.jsonl`).
- `AGENTS_TREE_COMPRESS_MIN`, `AGENTS_TREE_OUTPUT_DIR`, `AGENTS_TREE_COMPRESS_LOG`: threshold, saved-output directory and log of `compress`.

## Caveats

- Costs are estimates (`~$`) from flat API rates (rates of the Claude API model table of 2026-10-06; no batch, fast or priority pricing). Models without a known rate are excluded and counted, never priced as 0.
- "Where tokens go" estimates sizes as characters / 4 and splits the input cost in proportion; thinking, images and compaction are not modelled (they land in "other").
- Test fixtures under `tests/fixtures/` are hand-written, not real captures.
