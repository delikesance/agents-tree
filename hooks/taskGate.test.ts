import { expect, test } from 'claude-code/testing'
import { editGate, stopGate } from './taskGate'
import { createStopGuard } from './taskGuard'
import type { Task, TaskState } from './taskTypes'

const CWD = '/repo'

const task = (overrides: Partial<Task>): Task => ({
  id: 't1',
  title: 'First',
  phase: 'active',
  deps: [],
  files: ['hooks/a.ts'],
  verify: 'bun test',
  attempts: 0,
  ...overrides,
})

const stateOf = (...tasks: Task[]): TaskState => ({ goal: 'g', rev: 1, tasks })

const loaderOf = (state: TaskState | undefined) => async () => state

test('editGate denies a file_path outside the active task', async () => {
  const verdict = await editGate(loaderOf(stateOf(task({}))), CWD, { file_path: `${CWD}/src/x.ts` })
  expect(verdict?.deny.includes('src/x.ts')).toBe(true)
})

test('editGate allows a file_path inside the active task', async () => {
  expect(await editGate(loaderOf(stateOf(task({}))), CWD, { file_path: `${CWD}/hooks/a.ts` })).toBeUndefined()
})

test('editGate checks path and notebook_path arguments too', async () => {
  const load = loaderOf(stateOf(task({})))
  expect((await editGate(load, CWD, { path: `${CWD}/src/y.ts` }))?.deny.includes('src/y.ts')).toBe(true)
  expect((await editGate(load, CWD, { notebook_path: `${CWD}/src/n.ipynb` }))?.deny.includes('src/n.ipynb')).toBe(true)
})

test('editGate allows calls without file arguments or without state', async () => {
  expect(await editGate(loaderOf(stateOf(task({}))), CWD, { command: 'ls' })).toBeUndefined()
  expect(await editGate(loaderOf(undefined), CWD, { file_path: `${CWD}/src/x.ts` })).toBeUndefined()
})

test('stopGate refuses while a task is open and counts the refusal', async () => {
  const guard = createStopGuard()
  const refusal = await stopGate(loaderOf(stateOf(task({ phase: 'pending' }))), guard)
  expect(refusal?.block.includes('t1')).toBe(true)
  expect(guard.count()).toBe(1)
})

test('stopGate lets the agent stop when every task is done or blocked', async () => {
  const guard = createStopGuard()
  expect(await stopGate(loaderOf(stateOf(task({ phase: 'done' }), task({ id: 't2', phase: 'blocked' }))), guard)).toBeUndefined()
  expect(guard.count()).toBe(0)
})

test('stopGate yields after the maximum refusals and then restarts the count', async () => {
  const guard = createStopGuard()
  const load = loaderOf(stateOf(task({})))
  expect(await stopGate(load, guard)).toBeDefined()
  expect(await stopGate(load, guard)).toBeDefined()
  expect(await stopGate(load, guard)).toBeUndefined()
  expect(guard.count()).toBe(0)
  expect(await stopGate(load, guard)).toBeDefined()
})
