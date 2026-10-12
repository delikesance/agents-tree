import { MAX_TASK_ATTEMPTS, activeTask, type Task, type TaskState } from './taskTypes'
import { addTask, blockTask, failTask, finishTask, planTasks, startTask, type TaskResult, unblockTask } from './taskstate'

export type TaskToolDeps = {
  load(): Promise<TaskState | undefined>
  save(state: TaskState): Promise<void>
  runVerify(cmd: string): Promise<{ code: number; output: string }>
}

type TaskInput = { id: string; title: string; deps?: string[]; files?: string[]; verify: string }

const EVIDENCE_TAIL_CHARS = 2000

const TASK_SCHEMA = {
  type: 'object',
  properties: {
    id: { type: 'string', description: 'Unique task id' },
    title: { type: 'string', description: 'Short task title' },
    deps: { type: 'array', items: { type: 'string' }, description: 'Ids of tasks that must be done first' },
    files: { type: 'array', items: { type: 'string' }, description: 'Files the task touches' },
    verify: { type: 'string', description: 'Shell command that exits 0 when the task is done' },
  },
  required: ['id', 'title', 'verify'],
} as const

const TASK_ID_ONLY = {
  type: 'object',
  properties: { id: { type: 'string', description: 'Task id' } },
  required: ['id'],
} as const

export const TASK_TOOLS = [
  {
    name: 'task_plan',
    description: 'Create the task plan for the goal, replacing any existing plan. Tasks run in dependency order.',
    inputSchema: {
      type: 'object',
      properties: {
        goal: { type: 'string', description: 'Overall goal' },
        tasks: { type: 'array', items: TASK_SCHEMA, description: 'Tasks in plan order' },
      },
      required: ['goal', 'tasks'],
    },
  },
  {
    name: 'task_start',
    description: 'Start a pending task whose deps are done. Only one task may be active at a time.',
    inputSchema: TASK_ID_ONLY,
  },
  {
    name: 'task_done',
    description: 'Run the active task verify command. Marks the task done on exit 0, otherwise counts a failed attempt.',
    inputSchema: { type: 'object', properties: {} },
  },
  {
    name: 'task_block',
    description: 'Mark a task blocked with a reason, for example when it needs the user.',
    inputSchema: {
      type: 'object',
      properties: {
        id: { type: 'string', description: 'Task id' },
        reason: { type: 'string', description: 'Why the task is blocked' },
      },
      required: ['id', 'reason'],
    },
  },
  {
    name: 'task_unblock',
    description: 'Return a blocked task to pending with its attempts reset, so it can be started again.',
    inputSchema: TASK_ID_ONLY,
  },
  {
    name: 'task_add',
    description: 'Add a pending task to the current plan.',
    inputSchema: TASK_SCHEMA,
  },
] as const

const toTask = (input: TaskInput): Task => ({
  id: input.id,
  title: input.title,
  phase: 'pending',
  deps: input.deps ?? [],
  files: input.files ?? [],
  verify: input.verify,
  attempts: 0,
})

const isError = (result: TaskResult): result is { error: string } => 'error' in result

const tail = (output: string): string => output.slice(-EVIDENCE_TAIL_CHARS)

async function persist(deps: TaskToolDeps, result: TaskResult, message: string): Promise<string> {
  if (isError(result)) return result.error
  await deps.save(result)
  return message
}

async function completeActive(state: TaskState, deps: TaskToolDeps): Promise<string> {
  const active = activeTask(state)
  if (!active) return 'no active task'
  const { code, output } = await deps.runVerify(active.verify)
  const evidence = tail(output)
  if (code === 0) return persist(deps, finishTask(state, active.id, evidence), `done ${active.id}`)
  const result = failTask(state, active.id, evidence)
  if (isError(result)) return result.error
  const updated = result.tasks.find(task => task.id === active.id)
  await deps.save(result)
  if (updated?.phase === 'blocked') return `blocked: ask the user (${active.id} failed ${MAX_TASK_ATTEMPTS} times)`
  return `verify failed for ${active.id}, attempts left: ${MAX_TASK_ATTEMPTS - (updated?.attempts ?? 0)}`
}

async function runWithState(
  name: string,
  input: Record<string, unknown>,
  state: TaskState,
  deps: TaskToolDeps,
): Promise<string> {
  switch (name) {
    case 'task_start': {
      const id = String(input.id)
      return persist(deps, startTask(state, id), `started ${id}`)
    }
    case 'task_done':
      return completeActive(state, deps)
    case 'task_block': {
      const id = String(input.id)
      return persist(deps, blockTask(state, id, String(input.reason)), `blocked ${id}`)
    }
    case 'task_unblock': {
      const id = String(input.id)
      return persist(deps, unblockTask(state, id), `unblocked ${id}`)
    }
    case 'task_add': {
      const task = toTask(input as TaskInput)
      return persist(deps, addTask(state, task), `added ${task.id}`)
    }
    default:
      return `unknown task tool ${name}`
  }
}

export async function handleTaskTool(
  name: string,
  input: Record<string, unknown>,
  deps: TaskToolDeps,
): Promise<string> {
  if (name === 'task_plan') {
    const tasks = (input.tasks as TaskInput[]).map(toTask)
    return persist(deps, planTasks(String(input.goal), tasks), `plan saved with ${tasks.length} tasks`)
  }
  const state = await deps.load()
  if (!state) return 'no task plan: call task_plan first'
  return runWithState(name, input, state, deps)
}
