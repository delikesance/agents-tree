# agent-graph
TypeScript/TSX plugin for the host TUI. Host API types: `.claude-plugin/types/claude-code/index.d.ts`.
- Entry: `hooks/register.tsx`. Keep new code in small files under `hooks/`; file basenames must be distinct (`./x` resolves .tsx before .ts).
- `Button` children may only be strings/`Text`; use `$.ui.invalidate('ui.render')` to redraw after state changes.
- Tests: see README (bun with a temporary `claude-code/testing` shim, removed afterwards).
