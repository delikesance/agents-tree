import { expect, test } from 'claude-code/testing'
import { spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { hostTaskExec, hostTaskIo } from './taskIo'
import type { TaskRuntime } from './taskIo'
import { serveTaskCall, taskEditDeny, taskSections } from './taskServe'
import { STATE_FILE } from './taskStore'
import { TASKS_STORE_KEY } from './taskTypes'

const nodeRun = async (argv: string[], cwd?: string) => {
  const { status, stdout, stderr } = spawnSync(argv[0], argv.slice(1), { cwd, encoding: 'utf8' })
  return { exitCode: status ?? 1, stdout, stderr }
}
const nodeFs = {
  exists: async (path: string) => existsSync(path),
  read: (path: string) => readFile(path, 'utf8'),
  write: (path: string, text: string) => writeFile(path, text, 'utf8'),
}
const runtimeWith = (setStore: TaskRuntime['setStore'] = async () => undefined): TaskRuntime => ({
  io: hostTaskIo(nodeFs, nodeRun),
  exec: hostTaskExec(nodeRun),
  setStore,
})

const PREFIX = 'mcp__hive__'
const planCall = {
  tool: `${PREFIX}task_plan`,
  goal: 'ship',
  tasks: [{ id: 'a', title: 'A', verify: 'true' }],
}

test('serveTaskCall persists the plan to the state file and mirrors it to the store', async () => {
  const cwd = await mkdtemp(join(tmpdir(), 'hive-serve-'))
  const stored = new Map<string, unknown>()
  const runtime = runtimeWith(async (key, value) => void stored.set(key, value))
  const served = await serveTaskCall(runtime, cwd, planCall, PREFIX)
  expect(served.result).toContain('plan saved')
  const onDisk = JSON.parse(await readFile(join(cwd, STATE_FILE), 'utf8'))
  expect(onDisk.goal).toBe('ship')
  expect((stored.get(TASKS_STORE_KEY) as { goal: string }).goal).toBe('ship')
})

test('taskSections is empty without a project directory or state', async () => {
  expect(await taskSections(runtimeWith(), undefined)).toEqual([])
  expect(await taskSections(runtimeWith(), await mkdtemp(join(tmpdir(), 'hive-empty-')))).toEqual([])
})

test('taskEditDeny allows edits when no task is active', async () => {
  const cwd = await mkdtemp(join(tmpdir(), 'hive-edit-'))
  expect(await taskEditDeny(runtimeWith(), cwd, { file_path: join(cwd, 'x.ts') })).toBeUndefined()
  expect(await taskEditDeny(runtimeWith(), undefined, { file_path: 'x.ts' })).toBeUndefined()
})
