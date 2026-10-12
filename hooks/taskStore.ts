import type { TaskToolDeps } from './taskTools'
import type { TaskState } from './taskTypes'

export type TaskIo = {
  read(path: string): Promise<string | undefined>
  write(path: string, content: string): Promise<void>
  rename(from: string, to: string): Promise<void>
}

export type TaskExec = (cmd: string, cwd: string) => Promise<{ code: number; output: string }>

export const STATE_FILE = '.claudestate'
const TEMP_FILE = `${STATE_FILE}.tmp`

const statePath = (cwd: string) => `${cwd}/${STATE_FILE}`

const isTaskState = (value: unknown): value is TaskState => {
  if (typeof value !== 'object' || value === null) return false
  const candidate = value as Partial<TaskState>
  return typeof candidate.goal === 'string' && typeof candidate.rev === 'number' && Array.isArray(candidate.tasks)
}

export async function loadTaskState(io: TaskIo, cwd: string): Promise<TaskState | undefined> {
  const text = await io.read(statePath(cwd))
  if (text === undefined) return undefined
  try {
    const parsed: unknown = JSON.parse(text)
    return isTaskState(parsed) ? parsed : undefined
  } catch {
    return undefined
  }
}

export async function saveTaskState(io: TaskIo, cwd: string, state: TaskState): Promise<void> {
  const onDisk = await loadTaskState(io, cwd)
  if (onDisk && onDisk.rev >= state.rev) {
    throw new Error(`stale task state: disk rev ${onDisk.rev} >= ${state.rev}`)
  }
  const tempPath = `${cwd}/${TEMP_FILE}`
  await io.write(tempPath, JSON.stringify(state))
  await io.rename(tempPath, statePath(cwd))
}

export function createTaskToolDeps(io: TaskIo, cwd: string, exec: TaskExec): TaskToolDeps {
  return {
    load: () => loadTaskState(io, cwd),
    save: state => saveTaskState(io, cwd, state),
    runVerify: cmd => exec(cmd, cwd),
  }
}
