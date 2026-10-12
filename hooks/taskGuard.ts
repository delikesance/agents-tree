import { covers, relativeTo } from './skillGate'
import { activeTask, type Task, type TaskState } from './taskTypes'

export type GuardDeny = { deny: string }

const DEFAULT_MAX_REFUSALS = 2

const isOpen = (task: Task) => task.phase === 'active' || task.phase === 'pending'

const describe = (task: Task) => `${task.id} (${task.title})`

export const guardEdit = (state: TaskState | undefined, cwd: string, files: string[]): GuardDeny | undefined => {
  const active = activeTask(state)
  if (!active) return undefined
  const touched = files.map(file => relativeTo(cwd, file)).filter((file): file is string => !!file)
  const outside = touched.find(file => !active.files.some(allowed => covers(allowed, file)))
  if (!outside) return undefined
  return { deny: `Task ${active.id} may only edit ${active.files.join(', ')}; ${outside} is outside it.` }
}

export const guardStop = (state: TaskState | undefined, refusals: number, maxRefusals = DEFAULT_MAX_REFUSALS): GuardDeny | undefined => {
  if (refusals >= maxRefusals) return undefined
  const open = state?.tasks.filter(isOpen) ?? []
  if (open.length === 0) return undefined
  const current = activeTask(state) ?? open[0]
  return { deny: `Task ${describe(current)} is not finished; complete or block it before stopping.` }
}

export const createStopGuard = () => {
  let refusals = 0
  return {
    refuse: () => ++refusals,
    count: () => refusals,
    reset: () => {
      refusals = 0
    },
  }
}
