import { type Task, type TaskState } from './taskTypes'

export const TASK_SECTION_ID = 'hive:tasks'

const countPhase = (state: TaskState, phase: Task['phase']) => state.tasks.filter(task => task.phase === phase).length

const countsLine = (state: TaskState) =>
  `${countPhase(state, 'pending')} pending, ${countPhase(state, 'done')} done, ${countPhase(state, 'blocked')} blocked`

const depLines = (state: TaskState, task: Task) =>
  task.deps.map(depId => {
    const dep = state.tasks.find(candidate => candidate.id === depId)
    return dep ? `  - ${dep.id} [${dep.phase}]` : `  - ${depId} [unknown]`
  })

const taskLines = (state: TaskState, task: Task) => [
  `Active task: ${task.id} ${task.title}`,
  `Files: ${task.files.join(', ') || 'none'}`,
  `Verify: ${task.verify}`,
  `Attempts: ${task.attempts}`,
  'Direct deps:',
  ...(task.deps.length ? depLines(state, task) : ['  - none']),
]

export const renderActiveTask = (state: TaskState | undefined): string | undefined => {
  if (!state) return undefined
  const active = state.tasks.find(task => task.phase === 'active')
  const body = active ? taskLines(state, active) : ['Active task: none']
  return [`Goal: ${state.goal}`, ...body, countsLine(state)].join('\n')
}

export const taskSection = (state: TaskState | undefined) => {
  const text = renderActiveTask(state)
  return text === undefined ? undefined : { id: TASK_SECTION_ID, text, scope: 'session' as const }
}

export const progressView = (state: TaskState | undefined) => {
  const tasks = state?.tasks ?? []
  return {
    done: tasks.filter(task => task.phase === 'done').length,
    total: tasks.length,
    steps: tasks.map(task => task.title),
  }
}
