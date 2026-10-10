export type AgentUsage = {
  model: string
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
  cost: number
  steps: number
}

export type TodoItem = { content: string; status: string }

export type GoalProgress = { done: number; total: number }

declare module 'claude-code' {
  interface PluginState {
    'agent-graph': { usage: Record<string, AgentUsage>; activity: Record<string, string>; goals: Record<string, string>; models: Record<string, string>; routes: Record<string, string>; progress: Record<string, GoalProgress>; todos: Record<string, TodoItem[]>; mainFinished: boolean; trimmed: number }
  }
}
