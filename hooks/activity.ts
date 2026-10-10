import type { GoalProgress } from '../types'

const ACTIVITY_MAX = 44
const ACTIVITY_ARGS = ['description', 'file_path', 'command', 'pattern', 'path', 'url', 'query']
export const describeCall = (call: Record<string, unknown>) => {
  const key = ACTIVITY_ARGS.find(k => typeof call[k] === 'string' && call[k])
  const text = `${call.tool}${key ? ` ${call[key]}` : ''}`.replace(/\s+/g, ' ')
  return text.length > ACTIVITY_MAX ? `${text.slice(0, ACTIVITY_MAX - 1)}…` : text
}
export const todoProgress = (todos: readonly { status: string }[]): GoalProgress => ({
  done: todos.filter(t => t.status === 'completed').length,
  total: todos.length,
})
export type Todo = { content: string; status: string }
export const stepsToTodos = (steps: readonly string[], done: number): Todo[] =>
  steps.map((content, index) => ({ content, status: index < done ? 'completed' : index === done ? 'in_progress' : 'pending' }))
export const toTodos = (todos: readonly Todo[]): Todo[] => todos.map(({ content, status }) => ({ content, status }))
export const TASK_STATUS_DONE = 'completed'
export const countTask = (current: GoalProgress | undefined, { done = 0, total = 0 }: Partial<GoalProgress>): GoalProgress => ({
  done: (current?.done ?? 0) + done,
  total: (current?.total ?? 0) + total,
})
