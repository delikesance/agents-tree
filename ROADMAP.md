# Roadmap

Priorities follow one rule: show only what really happened, and keep the chat readable. Items are ordered within each phase.

## Now: make what exists trustworthy

- Replace the hand-written fixtures in `tests/fixtures/` with anonymised real captures (main + subagents + advisor + background agent) and pin parser/store behaviour on them.
- Verify `updatedToolOutput` acceptance per built-in tool shape for the `compress` PostToolUse hook; document which tools are safe, disable the others by default.
- Cost accuracy: model thinking tokens, images and compaction in "where tokens go" instead of dumping them in "other"; refresh rates in `internal/pricing` from the API model table and add a test that fails when a seen model has no rate.
- Robustness: tailers on truncated/rotated transcripts, very large sessions (parser and `Store.MaxMessages` memory), partial JSON lines.
- UI regressions: golden-frame tests with `agents-tree render` at several widths (narrow, 100, wide with side diagram).

## Next: day-to-day use

- Search in the chat (`/`) and jump between delegations, failures and reports.
- Per-agent filter: follow one subagent's thread; copy a message or tool output.
- Cost budget: threshold with a visible warning in the status line, per-session and per-agent.
- Session picker: sort/filter by project, cost, date; show cost and agent count per row.
- `agents-tree export <session>`: Markdown/HTML transcript of the chat view and a cost summary.
- Configurable keys and theme (file in `~/.config/agents-tree`).

## Later: analysis across sessions

- `agents-tree report`: cost and cache-hit trends across sessions, by project, model and tool.
- Compare two sessions (same task, different setup): tokens, cost, turns, composition.
- Actionable suggestions from `baseline`/`context` (oversized CLAUDE.md, noisy tool outputs, missing cache hits), each with the measured saving.
- Detect loops and wasteful patterns (repeated failing tool call, re-reads of the same file) and flag them in the details overlay.

## Exploring

- Hook-based live signals (permission prompts, notifications) instead of transcript inference only.
- Remote follow: tail a session on another machine over SSH.
- Packaged releases (goreleaser, Homebrew/AUR) and a `--version` with build info.

## Out of scope

- Writing into an interactive `claude` session opened elsewhere (one session is never driven from two places).
- Batch/fast/priority pricing models until the transcripts expose them.
