export const TASKS_STORE_KEY = 'tasks'
export const MAX_TASK_ATTEMPTS = 3

export type TaskPhase = 'pending' | 'active' | 'done' | 'blocked'

export type Task = {
  id: string
  title: string
  phase: TaskPhase
  deps: string[]
  files: string[]
  verify: string
  attempts: number
  evidence?: string
}

export type TaskState = { goal: string; rev: number; tasks: Task[] }

export const activeTask = (state: TaskState | undefined) => state?.tasks.find(task => task.phase === 'active')
