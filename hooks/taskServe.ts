import { createStopGuard } from './taskGuard'
import { editGate, stopGate } from './taskGate'
import { taskSection } from './taskInject'
import { handleTaskTool } from './taskTools'
import type { TaskRuntime } from './taskIo'
import { createTaskToolDeps, loadTaskState } from './taskStore'
import { TASKS_STORE_KEY } from './taskTypes'
import type { TaskState } from './taskTypes'

const stopGuard = createStopGuard()

const mirrorState = (runtime: TaskRuntime, state: TaskState | undefined) =>
  Promise.resolve()
    .then(() => runtime.setStore(TASKS_STORE_KEY, state))
    .catch(() => undefined)

export async function serveTaskCall(runtime: TaskRuntime, cwd: string, call: Record<string, unknown> & { tool: string }, prefix: string) {
  const deps = createTaskToolDeps(runtime.io, cwd, runtime.exec)
  const result = await handleTaskTool(call.tool.slice(prefix.length), call, deps)
  await mirrorState(runtime, await deps.load())
  return { result }
}

export const taskEditDeny = (runtime: TaskRuntime, cwd: string | undefined, call: Record<string, unknown>) =>
  cwd === undefined ? undefined : editGate(() => loadTaskState(runtime.io, cwd), cwd, call)

export const taskStopRefusal = (runtime: TaskRuntime, cwd: string | undefined) =>
  cwd === undefined ? undefined : stopGate(() => loadTaskState(runtime.io, cwd), stopGuard)

export const taskSections = async (runtime: TaskRuntime, cwd: string | undefined) => {
  if (cwd === undefined) return []
  const section = taskSection(await loadTaskState(runtime.io, cwd))
  return section ? [section] : []
}
