export const PROGRESS_SECTION = {
  id: 'hive:progress',
  text: 'Todo list (mandatory): before any other work on a task of 2 or more steps, write the full step list first: TodoWrite (or TaskCreate) when you have it, otherwise mcp__hive__progress with {done: 0, total, steps} (steps = one short label per step). Make it in the same message as your first working tool calls, so it costs no extra turn, but never start without it. Keep it current: mark a step done (same steps) in the message that starts the next one. Load mcp__hive__progress with ToolSearch if it is deferred.',
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
