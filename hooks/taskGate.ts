import { filesOf } from './callFiles'
import { guardEdit, guardStop, type GuardDeny, type createStopGuard } from './taskGuard'
import type { TaskState } from './taskTypes'

export type StopRefusal = { block: string }

export type StopGuard = ReturnType<typeof createStopGuard>

type LoadState = () => Promise<TaskState | undefined>

export const editGate = async (load: LoadState, cwd: string, call: Record<string, unknown>): Promise<GuardDeny | undefined> =>
  guardEdit(await load(), cwd, filesOf(call))

export const stopGate = async (load: LoadState, stopGuard: StopGuard): Promise<StopRefusal | undefined> => {
  const verdict = guardStop(await load(), stopGuard.count())
  if (!verdict) {
    stopGuard.reset()
    return undefined
  }
  stopGuard.refuse()
  return { block: verdict.deny }
}
