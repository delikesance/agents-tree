# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Project: agents-tree
Python/Textual TUI that shows Claude Code agents (main, subagents, advisor, JEV) as a live tree.
- Setup: `pip install -e '.[dev]'`; run: `agents-tree live [session.jsonl]` or `agents-tree replay <session.jsonl> --speed 4`; test: `python3 -m pytest -q` (one test: `pytest tests/test_store.py::test_tree_shape_and_status`).
- Data flow: `sources/` (transcript parser, live tailers, replay clock, hook file) -> normalized `Event`s (`model.py`) -> `Store.apply` (`store.py`, pure reducer) -> `ui/render.py` (Rich renderables) -> `ui/app.py` (Textual). Live and replay share the Store; only the source differs.
- Transcripts: `~/.claude/projects/<proj>/<session>.jsonl`; subagent transcripts are read from `<session>/subagents/agent-<id>.jsonl` (layout verified on a real run), with `agent-<id>.meta.json` giving `toolUseId`. The tool_use id of an `Agent`/`Task` call is the node id; `meta.json` / `toolUseResult.agentId` become aliases (can arrive before the node: `Store._pending_alias`). `Store._claim` is only a fallback heuristic.
- Hooks (optional): `hooks/emit.py` appends SubagentStart/Stop events to `$AGENTS_TREE_EVENTS` (default `~/.claude/agents-tree-events.jsonl`); wiring them in `settings.json` is manual. Prices in `pricing.py` are estimates (override via `$AGENTS_TREE_PRICING`).
- Session picker: `s` in the app (or `agents-tree live --pick`, `agents-tree sessions`) lists `~/.claude/projects/*/*.jsonl` (`sources/live.py: list_sessions`, `ui/picker.py`); choosing one resets the Store and re-reads that session. Hook events carry `session` and are filtered per session.
- Show only what really happened: advisor/JEV panels appear only after a call. Live view is "active" by default: only agents `Store.state == running` are drawn (`Store.visible_children`; finished, failed and silent agents vanish, ancestors of a running agent stay); `h` toggles the full history, and replay starts in "all". A subagent with no event for `STALE_SECS` (default 120s, env `AGENTS_TREE_STALE`) counts as not running; `main` then reads `◌ idle`. Each running box shows its current tool (`AgentNode.activity`, from the last `tool_use` of its turn). Live uses wall-clock, replay uses event time. Costs are estimates (`~$`).
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
