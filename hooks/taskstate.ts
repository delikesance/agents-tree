import { MAX_TASK_ATTEMPTS, activeTask, type Task, type TaskState } from './taskTypes'

export type TaskResult = TaskState | { error: string }

const fail = (error: string): TaskResult => ({ error })

const findTask = (state: TaskState, id: string): Task | undefined => state.tasks.find(task => task.id === id)

const unknownTask = (id: string): TaskResult => fail(`unknown task ${id}`)

const withTasks = (state: TaskState, tasks: Task[]): TaskState => ({ ...state, rev: state.rev + 1, tasks })

const updateTask = (state: TaskState, id: string, change: Partial<Task>): TaskState =>
  withTasks(
    state,
    state.tasks.map(task => (task.id === id ? { ...task, ...change } : task)),
  )

const depsDone = (state: TaskState, task: Task): boolean =>
  task.deps.every(depId => findTask(state, depId)?.phase === 'done')

const freshError = (task: Task): string | undefined =>
  task.phase === 'pending' && task.attempts === 0
    ? undefined
    : `task ${task.id} must start pending with 0 attempts`

function duplicateIdError(tasks: Task[]): string | undefined {
  const ids = tasks.map(task => task.id)
  const duplicate = ids.find((id, index) => ids.indexOf(id) !== index)
  return duplicate === undefined ? undefined : `duplicate task id: ${duplicate}`
}

function unknownDepError(tasks: Task[]): string | undefined {
  const ids = new Set(tasks.map(task => task.id))
  const edge = tasks
    .flatMap(task => task.deps.map(dep => ({ taskId: task.id, dep })))
    .find(({ dep }) => !ids.has(dep))
  return edge === undefined ? undefined : `unknown dep ${edge.dep} for task ${edge.taskId}`
}

function cycleError(tasks: Task[]): string | undefined {
  const depsOf = new Map(tasks.map(task => [task.id, task.deps]))
  const onPath = new Set<string>()
  const cleared = new Set<string>()
  const visit = (id: string): string | undefined => {
    if (cleared.has(id)) return undefined
    if (onPath.has(id)) return `dependency cycle at ${id}`
    onPath.add(id)
    const cycle = (depsOf.get(id) ?? []).map(visit).find(result => result !== undefined)
    onPath.delete(id)
    cleared.add(id)
    return cycle
  }
  return tasks.map(task => visit(task.id)).find(result => result !== undefined)
}

function graphError(tasks: Task[]): string | undefined {
  return duplicateIdError(tasks) ?? unknownDepError(tasks) ?? cycleError(tasks)
}

export function planTasks(goal: string, tasks: Task[]): TaskResult {
  const error = graphError(tasks) ?? tasks.map(freshError).find(result => result !== undefined)
  return error === undefined ? { goal, rev: 0, tasks: tasks.map(task => ({ ...task })) } : fail(error)
}

export function startTask(state: TaskState, id: string): TaskResult {
  const task = findTask(state, id)
  if (!task) return unknownTask(id)
  if (task.phase !== 'pending') return fail(`task ${id} is ${task.phase}, not pending`)
  if (!depsDone(state, task)) return fail(`task ${id} has unfinished deps`)
  const active = activeTask(state)
  if (active) return fail(`task ${active.id} is already active`)
  return updateTask(state, id, { phase: 'active' })
}

function onActiveTask(state: TaskState, id: string, change: (task: Task) => Partial<Task>): TaskResult {
  const task = findTask(state, id)
  if (!task) return unknownTask(id)
  if (task.phase !== 'active') return fail(`task ${id} is ${task.phase}, not active`)
  return updateTask(state, id, change(task))
}

export function finishTask(state: TaskState, id: string, evidence: string): TaskResult {
  return onActiveTask(state, id, () => ({ phase: 'done', evidence }))
}

export function failTask(state: TaskState, id: string, output: string): TaskResult {
  return onActiveTask(state, id, task => {
    const attempts = task.attempts + 1
    return {
      attempts,
      evidence: output,
      phase: attempts >= MAX_TASK_ATTEMPTS ? 'blocked' : 'active',
    }
  })
}

export function blockTask(state: TaskState, id: string, reason: string): TaskResult {
  const task = findTask(state, id)
  if (!task) return unknownTask(id)
  if (task.phase === 'done' || task.phase === 'blocked') return fail(`task ${id} is already ${task.phase}`)
  return updateTask(state, id, { phase: 'blocked', evidence: reason })
}

export function unblockTask(state: TaskState, id: string): TaskResult {
  const task = findTask(state, id)
  if (!task) return unknownTask(id)
  if (task.phase !== 'blocked') return fail(`task ${id} is ${task.phase}, not blocked`)
  return updateTask(state, id, { phase: 'pending', attempts: 0 })
}

export function addTask(state: TaskState, task: Task): TaskResult {
  if (findTask(state, task.id)) return fail(`duplicate task id: ${task.id}`)
  const error = freshError(task) ?? graphError([...state.tasks, task])
  return error === undefined ? withTasks(state, [...state.tasks, task]) : fail(error)
}

export const nextRunnable = (state: TaskState): Task[] =>
  state.tasks.filter(task => task.phase === 'pending' && depsDone(state, task))

export function assertRev(state: TaskState, expectedRev: number): string | undefined {
  return state.rev === expectedRev ? undefined : `stale state: expected rev ${expectedRev}, found ${state.rev}`
}
