# agent-graph

In-TUI plugin that shows the agents of a session (tree, cost, quotas, activity) in a side pane.

## Interactive mode
- Each agent card has an `ouvrir ▸` button: it opens that agent's live thread (messages and tool calls).
- The reply box sends a message to that agent (`$.session.send`), which also resumes a finished one.
- `← retour` goes back to the list.

## Layout
- `hooks/register.tsx` registers hooks, tools and the pane.
- `hooks/panel.tsx` list cards; `hooks/thread.tsx` thread view; `hooks/threadLines.ts` message to line mapping; `hooks/select.ts` selected agent.
- `preview/print-panel.tsx` prints the list as ANSI.

## Tests
Tests import `claude-code/testing` (provided by the host). Locally, put `export { expect, test } from 'bun:test'` in `node_modules/claude-code/testing.ts`, run `bun test hooks/`, then delete `node_modules`.
