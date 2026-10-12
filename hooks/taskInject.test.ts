import { expect, test } from 'claude-code/testing'

import { progressView, renderActiveTask, taskSection } from './taskInject'
import { type TaskState } from './taskTypes'

const state: TaskState = {
  goal: 'Ship tasks',
  rev: 1,
  tasks: [
    { id: 't1', title: 'Types', phase: 'done', deps: [], files: ['hooks/taskTypes.ts'], verify: 'bun test', attempts: 1 },
    { id: 't2', title: 'Inject', phase: 'active', deps: ['t1', 't3'], files: ['hooks/taskInject.ts'], verify: 'bun test hooks/taskInject.test.ts', attempts: 2 },
    { id: 't3', title: 'Store', phase: 'blocked', deps: [], files: [], verify: 'tsc', attempts: 0 },
    { id: 't4', title: 'Later', phase: 'pending', deps: ['t2'], files: [], verify: 'tsc', attempts: 0 },
  ],
}

test('renderActiveTask is undefined without state', () => {
  expect(renderActiveTask(undefined)).toBe(undefined)
})

test('renderActiveTask shows goal, active task, direct deps and counts', () => {
  expect(renderActiveTask(state)).toBe(
    [
      'Goal: Ship tasks',
      'Active task: t2 Inject',
      'Files: hooks/taskInject.ts',
      'Verify: bun test hooks/taskInject.test.ts',
      'Attempts: 2',
      'Direct deps:',
      '  - t1 [done]',
      '  - t3 [blocked]',
      '1 pending, 1 done, 1 blocked',
    ].join('\n'),
  )
})

test('renderActiveTask reports no active task and no deps', () => {
  const idle: TaskState = { goal: 'G', rev: 0, tasks: [{ id: 'a', title: 'A', phase: 'pending', deps: [], files: [], verify: 'v', attempts: 0 }] }
  expect(renderActiveTask(idle)).toBe(['Goal: G', 'Active task: none', '1 pending, 0 done, 0 blocked'].join('\n'))
})

test('renderActiveTask marks unknown deps', () => {
  const dangling: TaskState = { goal: 'G', rev: 0, tasks: [{ id: 'a', title: 'A', phase: 'active', deps: ['gone'], files: [], verify: 'v', attempts: 0 }] }
  expect(renderActiveTask(dangling)).toContain('  - gone [unknown]')
})

test('taskSection matches the progress section shape', () => {
  expect(taskSection(undefined)).toBe(undefined)
  expect(taskSection(state)?.id).toBe('hive:tasks')
  expect(taskSection(state)?.scope).toBe('session')
  expect(taskSection(state)?.text).toBe(renderActiveTask(state))
})

test('progressView counts done tasks and lists titles', () => {
  expect(progressView(state)).toEqual({ done: 1, total: 4, steps: ['Types', 'Inject', 'Store', 'Later'] })
  expect(progressView(undefined)).toEqual({ done: 0, total: 0, steps: [] })
})
