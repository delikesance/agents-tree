# agents-tree

Terminal UI for Claude Code sessions. It follows a session transcript live (or replays one) and shows:

- a chat: one box per message (you, Claude, tool calls, delegations to subagents and their reports);
- an agent rail: the agents running right now (main, subagents, advisor, JEV forks), each with its current tool;
- an optional tree view of the agents (`t`);
- cost with prompt-cache accounting (uncached input, cache writes and reads, output, hit rate, savings);
- "where tokens go": which parts of the context (system prompt, tool outputs per tool, your messages...) are re-sent on every request;
- a message box that sends your text to Claude Code through a `claude -p` process resumed on the followed session.

## Install

Go 1.26 or newer.

```
go install github.com/delikesance/agents-tree/cmd/agents-tree@latest
# or, from a checkout:
go build -o agents-tree ./cmd/agents-tree
```

## Commands

```
agents-tree live [session.jsonl]    follow a session (default: latest of this project)
      --pick                        choose the session in the app
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

`q` quit, `s` switch session, `i` / `enter` write a message (`enter` sends, `alt+enter` new line, `esc` back to the chat),
`f` filter by agent, `e` unfold long messages, `t` chat / tree view, `h` finished agents in the rail and tree (live starts on "active"),
`ctrl+k` stop Claude, `end` follow the bottom, arrows / `pgup` / `pgdown` / `home` scroll, `ctrl+c` quit.
Replay only: `space` pause, `+` / `-` speed.

## Permissions

Messages are sent with `claude -p --input-format stream-json ... --resume <session>`. The default `--permissions all` passes
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
