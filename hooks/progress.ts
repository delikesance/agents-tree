export const PROGRESS_SECTION = {
  id: 'agent-graph:progress',
  text: 'Progress bar (mandatory): for any task of 3 or more steps, and whenever you have no task-list tool (TodoWrite/TaskCreate), call mcp__agent-graph__progress with {done: 0, total, steps} where steps lists every step as a short label, in the same message as your first working tool calls; update it (same steps) when a step finishes, always batched with the next step's tool calls and never as a turn of its own. Load it with ToolSearch if it is deferred.',
  scope: 'session',
} as const

export const PROGRESS_TOOL = {
  name: 'progress',
  description: 'Report how many steps of the current task are done so the agent box shows a filling progress background.',
  inputSchema: {
    type: 'object',
    properties: {
      done: { type: 'integer', description: 'Steps finished' },
      total: { type: 'integer', description: 'Steps planned in total' },
      steps: { type: 'array', items: { type: 'string' }, description: 'Short label of each step, in order; shown as a checklist' },
    },
    required: ['done', 'total'],
  },
} as const
