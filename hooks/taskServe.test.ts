import { expect, test } from 'claude-code/testing'
import { mkdtemp, readFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { serveTaskCall, taskEditDeny, taskSections } from './taskServe'
import { STATE_FILE } from './taskStore'
import { TASKS_STORE_KEY } from './taskTypes'

const PREFIX = 'mcp__hive__'
const planCall = {
  tool: `${PREFIX}task_plan`,
  goal: 'ship',
  tasks: [{ id: 'a', title: 'A', verify: 'true' }],
}

test('serveTaskCall persists the plan to the state file and mirrors it to the store', async () => {
  const cwd = await mkdtemp(join(tmpdir(), 'hive-serve-'))
  const stored = new Map<string, unknown>()
  const host = { store: { set: async (key: string, value: unknown) => void stored.set(key, value) } }
  const served = await serveTaskCall(host as never, cwd, planCall, PREFIX)
  expect(served.result).toContain('plan saved')
  const onDisk = JSON.parse(await readFile(join(cwd, STATE_FILE), 'utf8'))
  expect(onDisk.goal).toBe('ship')
  expect((stored.get(TASKS_STORE_KEY) as { goal: string }).goal).toBe('ship')
})

test('taskSections is empty without a project directory or state', async () => {
  expect(await taskSections(undefined)).toEqual([])
  expect(await taskSections(await mkdtemp(join(tmpdir(), 'hive-empty-')))).toEqual([])
})

test('taskEditDeny allows edits when no task is active', async () => {
  const cwd = await mkdtemp(join(tmpdir(), 'hive-edit-'))
  expect(await taskEditDeny(cwd, { file_path: join(cwd, 'x.ts') })).toBeUndefined()
  expect(await taskEditDeny(undefined, { file_path: 'x.ts' })).toBeUndefined()
})
