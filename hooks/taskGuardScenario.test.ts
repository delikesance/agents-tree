import { expect, test } from 'claude-code/testing'
import { createStopGuard, guardEdit, guardStop } from './taskGuard'
import { lowGainSlugs, skillGain } from './skillMetric'
import type { SkillRecord } from './skillTypes'
import type { Task, TaskState } from './taskTypes'

const CWD = '/repo'

const task = (overrides: Partial<Task>): Task => ({
  id: 't1',
  title: 'Edit guard',
  phase: 'active',
  deps: [],
  files: ['hooks/taskGuard.ts'],
  verify: 'bun test',
  attempts: 0,
  ...overrides,
})

const stateOf = (...tasks: Task[]): TaskState => ({ goal: 'g', rev: 0, tasks })

test('guardEdit denies a file outside the active task and names the task and allowed files', () => {
  const verdict = guardEdit(stateOf(task({})), CWD, [`${CWD}/hooks/other.ts`])
  expect(verdict?.deny).toContain('t1')
  expect(verdict?.deny).toContain('hooks/taskGuard.ts')
  expect(verdict?.deny).toContain('hooks/other.ts')
})

test('guardEdit allows the declared file', () => {
  expect(guardEdit(stateOf(task({})), CWD, [`${CWD}/hooks/taskGuard.ts`])).toBeUndefined()
})

test('guardEdit allows any file under a declared directory', () => {
  const state = stateOf(task({ files: ['hooks/'] }))
  expect(guardEdit(state, CWD, [`${CWD}/hooks/deep/x.ts`])).toBeUndefined()
})

test('guardEdit ignores files outside cwd', () => {
  expect(guardEdit(stateOf(task({})), CWD, ['/elsewhere/hooks/other.ts'])).toBeUndefined()
})

test('guardEdit returns undefined without state or without an active task', () => {
  expect(guardEdit(undefined, CWD, [`${CWD}/hooks/other.ts`])).toBeUndefined()
  expect(guardEdit(stateOf(task({ phase: 'pending' })), CWD, [`${CWD}/hooks/other.ts`])).toBeUndefined()
})

test('guardStop denies while tasks are open, then lets through after two refusals', () => {
  const state = stateOf(task({ phase: 'pending' }))
  const guard = createStopGuard()
  expect(guardStop(state, guard.count())?.deny).toContain('t1')
  guard.refuse()
  expect(guardStop(state, guard.count())).toBeDefined()
  guard.refuse()
  expect(guardStop(state, guard.count())).toBeUndefined()
})

test('createStopGuard reset restores refusals', () => {
  const guard = createStopGuard()
  guard.refuse()
  guard.refuse()
  guard.reset()
  expect(guard.count()).toBe(0)
  expect(guardStop(stateOf(task({ phase: 'pending' })), guard.count())).toBeDefined()
})

test('guardStop returns undefined once every task is done or blocked', () => {
  const state = stateOf(task({ id: 'a', phase: 'done' }), task({ id: 'b', phase: 'blocked' }))
  expect(guardStop(state, 0)).toBeUndefined()
})

const record = (overrides: Partial<SkillRecord>): SkillRecord => ({
  slug: 's',
  status: 'active',
  tags: [],
  sessions: [],
  signals: [],
  citedPaths: [],
  headCommit: 'h',
  createdAt: 0,
  idleSessions: 0,
  uses: 3,
  cleanUses: 3,
  ...overrides,
})

test('skillGain computes cost reduction and lowGainSlugs filters by it', () => {
  const helpful = record({
    slug: 'helpful',
    outcomes: {
      with: { sessions: 2, turns: 4, corrections: 0 },
      without: { sessions: 2, turns: 10, corrections: 2 },
    },
  })
  const useless = record({
    slug: 'useless',
    outcomes: {
      with: { sessions: 1, turns: 9, corrections: 1 },
      without: { sessions: 1, turns: 5, corrections: 0 },
    },
  })
  expect(skillGain(helpful)).toBe(4)
  expect(skillGain(useless)).toBe(-5)
  expect(lowGainSlugs({ helpful, useless })).toEqual(['useless'])
})

test('skillGain is undefined below the minimum uses or without outcomes', () => {
  expect(skillGain(record({ uses: 2 }))).toBeUndefined()
  expect(skillGain(record({}))).toBeUndefined()
})
