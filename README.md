# HIVE

In-TUI plugin that shows the agents of a session (tree, cost, quotas, activity) in a side pane.

## Interactive mode
- Each agent card has an `ouvrir ▸` button: it opens that agent's live thread (messages and tool calls).
- The reply box sends a message to that agent (`$.session.send`), which also resumes a finished one.
- `← retour` goes back to the list.

## Layout
- `hooks/register.tsx` registers hooks, tools and the pane.
- `hooks/panel.tsx` list cards; `hooks/thread.tsx` thread view; `hooks/threadLines.ts` message to line mapping; `hooks/select.ts` selected agent.
- `preview/print-panel.tsx` prints the list as ANSI.

## Skill learning
HIVE watches the main session for repeated work and turns it into project skills.
- Signals: a retry that finally succeeded, a long exploration before the first edit, a user correction. Similar signals are grouped in a ledger kept in the plugin store (`skills`).
- A group seen in 3 sessions (`PROMOTE_MIN_SESSIONS`) is synthesized and written to `<project>/.claude/skills/<slug>/SKILL.md` (at most 20 live skills). Steps must cite a project file, otherwise the skill is refused. The first write to a cited file is denied until `/<slug>` is invoked (once per skill and session). A failed synthesis keeps the candidate and is retried, 3 attempts at most.
- Use is counted from the `skill.prompt` event; a use is clean when the next prompt is not a correction. A skill unused for 10 sessions (`ARCHIVE_AFTER_IDLE_SESSIONS`), or with fewer than half clean uses over 5 or more uses, is archived and its file removed.
- A skill whose cited files changed in git since it was written turns stale; revalidate it from the SKILLS section of the pane.
- Veto: `rejeter` in the SKILLS section deletes the file and the same pattern is never proposed again; `restaurer` brings a rejected or archived skill back as a candidate.
- `/hive discover` scans README.md sections and asks the model to propose non-obvious skills from a repo digest (hot files, co-changed groups, fix/revert commits, recurring corrections), then judges each on three criteria: multi-step (3+ steps), grounded (cites an existing file), new (no overlap with a ledger skill). It lists accepted and rejected candidates with the failed criteria; `/hive discover apply` synthesizes and writes the accepted ones (within the 20-skill cap).
- Skills written by hand are not in the ledger and are never touched.

## Tests
Tests import `claude-code/testing` (provided by the host). Locally, put `export { expect, test } from 'bun:test'` in `node_modules/claude-code/testing.ts`, run `bun test hooks/`, then delete `node_modules`.
