# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Project: agents-tree
Go/Bubble Tea v2 TUI (module `github.com/delikesance/agents-tree`, Go >= 1.26) that shows Claude Code agents (main, subagents, advisor, JEV) as a chat + agent rail + optional tree. The Go port replaced the earlier Python implementation.
- Setup: `go build ./...` (binary: `go build -o agents-tree ./cmd/agents-tree`); run: `agents-tree live [session.jsonl]` or `agents-tree replay <session.jsonl> --speed 4`; test: `go test ./...` (one test: `go test ./internal/store -run TestName`). Developer aid: `agents-tree render <session.jsonl> --width 100 --height 30 [--keys t,h]` prints one frame as ANSI text.
- Data flow: sources (`internal/transcript` parser, `internal/tail` live tailers, `internal/replay` clock, hook file read by `tail.HookTailer`) -> normalized `model.Event`s (`internal/model`) -> `store.Store.Apply` (`internal/store`, pure reducer, no I/O) -> `internal/ui` (Bubble Tea: `app.go` Model/Update/View, `messages.go` chat boxes, `rail.go` rail + cost/advisor/JEV/context panels, `tree.go`, `picker.go`, `theme.go`). Live and replay share the Store; only the source differs. `cmd/agents-tree/main.go` is the CLI (`live`, `replay`, `sessions`, `context`, `hook`, `render`); its tests call `run(args, stdout, stderr)` without starting the TUI.
- Transcripts: `~/.claude/projects/<proj>/<session>.jsonl`; subagent transcripts are read from `<session>/subagents/agent-<id>.jsonl` (layout verified on a real run; `transcript.SessionFiles`), with `agent-<id>.meta.json` giving `toolUseId` (`transcript.MetaAlias`). The tool_use id of an `Agent`/`Task` call is the node id; `meta.json` / `toolUseResult.agentId` become aliases (can arrive before the node: `Store.pendingAlias`). `Store.claim` is only a fallback heuristic.
- Usage de-duplication: Claude Code writes one transcript line per content block and repeats the request's `usage` on each; `transcript.Parser` counts it once per `message.id` (repeats are flagged `Dup` and only update the agent's current tool). Counting every line overstated turns and cost by ~1.9x. `internal/composition` dedupes the same way.
- Hooks (optional): `agents-tree hook` (`runHook` in main.go) is a SubagentStart/SubagentStop command hook; it appends `tail.HookEvent` lines to `$AGENTS_TREE_EVENTS` (default `~/.claude/agents-tree-events.jsonl`), never prints, always exits 0; wiring it in `settings.json` is manual (README has the snippet). Hook events carry `session` and are filtered per session by `HookTailer`; `live --no-hooks` ignores the file.
- Cost model (`internal/pricing`, rates from the Claude API model table of 2026-10-06; override with `$AGENTS_TREE_PRICING`): per request, uncached input + cache writes (5m x1.25, 1h x2 via `usage.cache_creation`) + cache reads (model-specific: 0.1x default, 0.05x Opus 5.5, $0.25/MTok Fable 5.1) + output; Haiku 5.5 uses long-prompt rates above 100K prompt tokens; unknown models are excluded and counted (`pricing.TurnCost` ok=false, `AgentNode.UnpricedTurns`), never priced as 0. `Store.Summary()` gives the split, cache hit rate (`cache_read / prompt`) and savings vs. no cache; the cost panel and per-agent cards show them. Flat API rates only (no batch/fast/priority).
- Where tokens go (`internal/composition`, `agents-tree context [--recent N]`, "where tokens go" panel in the rail): input volume of each piece of content = its size x the requests that re-send it, split into context before the first message, tool calls, tool outputs per tool, user text, assistant text, other; sizes are chars/4 and money is the input cost split by volume (estimates).
- Session picker: `s` in the app (or `agents-tree live --pick`, `agents-tree sessions`) lists `~/.claude/projects/*/*.jsonl` (`sessions.List`, `internal/ui/picker.go`); choosing one resets the Store and re-reads that session (`Model.load`).
- Show only what really happened: advisor/JEV panels appear only after a call. Live view is "active" by default: only agents with `Store.State == running` are drawn (`Store.VisibleChildren`; finished, failed and silent agents vanish, ancestors of a running agent stay); `h` toggles the full history, and replay starts in "all". A subagent with no event for `store.StaleSecs` (default 120s, env `AGENTS_TREE_STALE`) counts as not running; `main` then reads idle. Each running card shows its current tool (`AgentNode.Activity`, from the last `tool_use` of its turn). Live uses wall-clock, replay uses event time (`Model.now()` returns 0 = newest event). Costs are estimates (`~$`).
- Chat (default view, `t` toggles the tree): the transcript parser also emits `MESSAGE` / `MSG_UPDATE` events (`model.Message`: user, assistant, tool, delegation, report, system); `Store.Messages` keeps the last `store.MaxMessages` (2000), deduped by id, tool status updated in place via `Rev`. `ui/messages.go` renders one box per message; the chat viewport re-renders only new/changed messages (cache keyed by a message signature) and follows the bottom unless scrolled up (`end` re-follows). `ui/rail.go` is the left rail (running agents only; `h` for all). Keys: `f` filter by agent, `e` unfold, `t` tree, `s` session, `i`/enter write, `ctrl+k` stop Claude. Subagent transcripts: the first user turn is the delegated prompt (shown by the delegation box, not duplicated), `SubagentHandback` is hidden (the report box replaces it). The chat keeps the whole history; only the rail/tree hide finished agents.
- Sending messages (`internal/sender`, `sender.Sender`): the TUI owns a `claude -p --input-format stream-json --output-format stream-json --verbose` subprocess (`--resume <id>` for the followed session, `--session-id <uuid>` for a new one; `BuildCommand`) and writes user messages to its stdin; replies reach the chat through the normal transcript tail, stdout is only read for `result` (turn end / errors). It cannot write into an interactive `claude` open elsewhere: never drive one session from two places. `--permissions all|accept-edits|plan` (default `all` = `--dangerously-skip-permissions`, chosen by the user; the UI shows a warning), `--no-send` hides the box, replay is read-only. `sender.CleanEnv` strips `CLAUDE_CODE_SESSION*`/`REMOTE*`/`CHILD*`/... from the child: a probe launched from inside a Claude Code session otherwise reuses that session's id. Tests use the stand-in `tests/fake_claude.py` (no network, no cost); never run the real `claude` from tests or agents.
- Fixtures in `tests/fixtures/` are hand-written minimal sessions, not real captures.

# Advisor checkpoints
- Call `advisor` before finalizing any multi-file architecture plan.
- If the same test or compiler error fails twice, call `advisor` before a third attempt.
- Before declaring a task complete or staging/committing, call `advisor` to audit the diff
  against the stated contract (scope, tests, no stray changes).

# Agent flow (follow this order)
1. Main session (Sonnet 5.5, high) plans and decides.
2. JEV fork layer: for cheap choices (which file, which tool, retry or stop) use the `jev_*` tools first, if they are loaded
   (check with ToolSearch; if absent, or JEV_API_KEY is missing, decide yourself).
3. Dispatcher: to delegate, call the `dispatcher` subagent (Haiku, medium) with the task. It returns a routing plan
   (which subagents, prompts, parallel groups). Do not count on subagents spawning subagents (a probe on 2026-10-09 showed
   workers had no Agent tool), so launch each planned run yourself, up to 3 in parallel, honoring the plan's parallel groups.
   Cost rule: before a worker starts on unfamiliar code, run an `explorer` (Haiku) first and paste its findings into the
   worker's prompt, so the Sonnet worker does not spend tokens on a wide read-only sweep.
4. Subagents (any agent in ~/.claude/agents; the set is open-ended):
   - `worker`: Sonnet 5.5, medium; edits + tests.
   - `explorer`: Haiku 5.5, medium; read code and AST, read-only.
   - `researcher`: Haiku 5.5, medium; look up docs and specs, read-only.
5. Back to the main session (high): review + verify the results before reporting or committing.

# JEV fast decisions (MCP `jev`)
- For cheap, low-stakes choices use the `jev_*` tools instead of deliberating: `jev_which_file` (which file to open first), `jev_which_tool` (which tool next), `jev_retry_or_stop` (after a failure repeats).
- Call `jev_retry_or_stop` when the same command or test fails twice, before a third attempt.
- If the result has `escalate: true` (low confidence), decide yourself or call `advisor`.
- Inputs go to TypeSafe: pass short summaries (paths, one-line errors), never file contents or secrets.
- Not for high-stakes calls (destructive actions, commits, architecture): use judgment and `advisor`.

# Cost routing (measured 2026-10-09; keep this section short, it is read every turn)
- Route by need: JEV for choices among named options, Haiku (`explorer`/`researcher`/`dispatcher`) for read-only work, Sonnet `worker` for edits + tests. Do mechanical edits (sed-able, many identical sites) yourself: a worker re-reading the repo costs more.
- Before a worker starts on code it does not know, run an `explorer` first and paste its findings into the worker prompt. Workers did not have the Agent tool in one probe (UNVERIFIED: that probe's tool list matched neither old nor new worker.md, so it may have used a stale definition); do not rely on it until re-probed after a restart.
- Size tasks small: a worker's cost grows with its turns (cache re-reads of a growing context): 63 turns ~ $0.9, 197 turns ~ $3.8. Split anything likely to pass ~100 turns, and cap the worker report at ~15 lines.
- Advisor budget for workers: at most 2 calls (once before committing to an approach, once before the final report), never two in a row. The only advisor catch measured came from the end-of-task call, so do not drop it; the lead still audits every diff before committing.
- Agent files in ~/.claude/agents use `effort:` (NOT `effortLevel:`, which is silently ignored); Haiku agents set `omitClaudeMd: true`. Docs say definitions reload without restart (UNVERIFIED here).
- Hooks in ~/.claude/hooks (wired in settings.json): `agent-guard.py` denies a subagent spawning anything but explorer/researcher; `read-nudge.py` reminds after 20 (subagent) / 30 (main) read-only calls. Both fail open.
- Measure, don't guess: `python3 ~/.claude/bin/agent-cost.py <tasks dir> [id prefixes]` gives turns, advisor calls and $ per subagent transcript (the advisor's own cost is only a range: it is not recorded).
- Backups of the pre-optimization config: ~/.claude/backups/tokens-routing-20261009-003005. JEV_API_KEY comes from ~/.bashrc (never write it in a file under version control).
