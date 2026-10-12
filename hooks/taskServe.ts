import type { CoreEngineInterface } from 'claude-code'
import { createStopGuard } from './taskGuard'
import { editGate, stopGate } from './taskGate'
import { taskSection } from './taskInject'
import { handleTaskTool } from './taskTools'
import { diskTaskIo, shellTaskExec } from './taskIo'
import { createTaskToolDeps, loadTaskState } from './taskStore'
import { TASKS_STORE_KEY } from './taskTypes'
import type { TaskState } from './taskTypes'

type TaskHost = Pick<CoreEngineInterface, 'store'>

const stopGuard = createStopGuard()

const mirrorState = (host: TaskHost, state: TaskState | undefined) =>
  Promise.resolve()
    .then(() => host.store.set(TASKS_STORE_KEY, state))
    .catch(() => undefined)

export async function serveTaskCall(host: TaskHost, cwd: string, call: Record<string, unknown> & { tool: string }, prefix: string) {
  const deps = createTaskToolDeps(diskTaskIo, cwd, shellTaskExec)
  const result = await handleTaskTool(call.tool.slice(prefix.length), call, deps)
  await mirrorState(host, await deps.load())
  return { result }
}

export const taskEditDeny = (cwd: string | undefined, call: Record<string, unknown>) =>
  cwd === undefined ? undefined : editGate(() => loadTaskState(diskTaskIo, cwd), cwd, call)

export const taskStopRefusal = (cwd: string | undefined) =>
  cwd === undefined ? undefined : stopGate(() => loadTaskState(diskTaskIo, cwd), stopGuard)

export const taskSections = async (cwd: string | undefined) => {
  if (cwd === undefined) return []
  const section = taskSection(await loadTaskState(diskTaskIo, cwd))
  return section ? [section] : []
}
