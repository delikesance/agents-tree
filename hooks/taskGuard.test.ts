import { expect, test } from 'claude-code/testing'
import { createStopGuard, guardEdit, guardStop } from './taskGuard'
import type { Task, TaskState } from './taskTypes'

const CWD = '/repo'

const task = (overrides: Partial<Task>): Task => ({
  id: 't1',
  title: 'First',
  phase: 'pending',
  deps: [],
  files: ['hooks/a.ts'],
  verify: 'bun test',
  attempts: 0,
  ...overrides,
})

const stateOf = (...tasks: Task[]): TaskState => ({ goal: 'g', rev: 1, tasks })

test('guardEdit allows edits when there is no state or no active task', () => {
  expect(guardEdit(undefined, CWD, [`${CWD}/x.ts`])).toBeUndefined()
  expect(guardEdit(stateOf(task({ phase: 'pending' })), CWD, [`${CWD}/x.ts`])).toBeUndefined()
})

test('guardEdit allows files inside the active task files', () => {
  const state = stateOf(task({ phase: 'active', files: ['hooks/a.ts', 'src/'] }))
  expect(guardEdit(state, CWD, [`${CWD}/hooks/a.ts`, `${CWD}/src/deep/b.ts`])).toBeUndefined()
})

test('guardEdit denies a touched file outside the active task and names task and allowed files', () => {
  const state = stateOf(task({ id: 't7', phase: 'active', files: ['hooks/a.ts'] }))
  const verdict = guardEdit(state, CWD, [`${CWD}/hooks/other.ts`])
  expect(verdict?.deny).toContain('t7')
  expect(verdict?.deny).toContain('hooks/a.ts')
  expect(verdict?.deny).toContain('hooks/other.ts')
})

test('guardEdit ignores files outside cwd', () => {
  const state = stateOf(task({ phase: 'active', files: ['hooks/a.ts'] }))
  expect(guardEdit(state, CWD, ['/elsewhere/x.ts'])).toBeUndefined()
})

test('guardStop allows finishing when no task is open', () => {
  expect(guardStop(undefined, 0)).toBeUndefined()
  expect(guardStop(stateOf(task({ phase: 'done' }), task({ id: 't2', phase: 'blocked' })), 0)).toBeUndefined()
})

test('guardStop denies while a task is pending and names the next task', () => {
  const state = stateOf(task({ id: 't1', phase: 'done' }), task({ id: 't2', title: 'Second', phase: 'pending' }))
  expect(guardStop(state, 0)?.deny).toContain('t2 (Second)')
})

test('guardStop prefers the active task in the message', () => {
  const state = stateOf(task({ id: 't2', title: 'Second', phase: 'pending' }), task({ id: 't1', title: 'First', phase: 'active' }))
  expect(guardStop(state, 0)?.deny).toContain('t1 (First)')
})

test('guardStop gives up after maxRefusals consecutive refusals', () => {
  const state = stateOf(task({ phase: 'active' }))
  expect(guardStop(state, 1)).toBeDefined()
  expect(guardStop(state, 2)).toBeUndefined()
  expect(guardStop(state, 1, 1)).toBeUndefined()
})

test('createStopGuard counts refusals and resets', () => {
  const guard = createStopGuard()
  expect(guard.refuse()).toBe(1)
  expect(guard.refuse()).toBe(2)
  expect(guard.count()).toBe(2)
  guard.reset()
  expect(guard.count()).toBe(0)
})
