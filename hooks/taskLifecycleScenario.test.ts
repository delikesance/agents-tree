import { expect, test } from 'claude-code/testing'
import { handleTaskTool, type TaskToolDeps } from './taskTools'
import { assertRev } from './taskstate'
import { MAX_TASK_ATTEMPTS, type TaskState } from './taskTypes'

const PLAN = {
  goal: 'ship scenario',
  tasks: [
    { id: 'A', title: 'first', files: ['hooks/a.ts'], verify: 'verify-a' },
    { id: 'B', title: 'second', deps: ['A'], files: ['hooks/b.ts'], verify: 'verify-b' },
    { id: 'C', title: 'third', deps: ['B'], files: ['hooks/c.ts'], verify: 'verify-c' },
  ],
}

function memoryDeps(exitCodes: number[]) {
  let stored: TaskState | undefined
  const deps: TaskToolDeps = {
    load: async () => stored,
    save: async state => {
      stored = state
    },
    runVerify: async cmd => ({ code: exitCodes.shift() ?? 1, output: `ran ${cmd}` }),
  }
  const current = () => {
    if (!stored) throw new Error('nothing saved')
    return stored
  }
  return { deps, current }
}

const taskOf = (state: TaskState, id: string) => state.tasks.find(task => task.id === id)

test('three failing verifies block task A and ask the user', async () => {
  const { deps, current } = memoryDeps([1, 1, 1])
  await handleTaskTool('task_plan', PLAN, deps)
  expect(await handleTaskTool('task_start', { id: 'A' }, deps)).toBe('started A')
  expect(await handleTaskTool('task_done', {}, deps)).toBe('verify failed for A, attempts left: 2')
  expect(await handleTaskTool('task_done', {}, deps)).toBe('verify failed for A, attempts left: 1')
  expect(await handleTaskTool('task_done', {}, deps)).toBe(`blocked: ask the user (A failed ${MAX_TASK_ATTEMPTS} times)`)
  const blocked = taskOf(current(), 'A')
  expect(blocked?.phase).toBe('blocked')
  expect(blocked?.attempts).toBe(MAX_TASK_ATTEMPTS)
})

test('chain A -> B -> C enforces deps, records evidence and keeps rev strictly increasing', async () => {
  const { deps, current } = memoryDeps([1, 0, 0])
  const revs: number[] = []
  const record = () => revs.push(current().rev)

  await handleTaskTool('task_plan', PLAN, deps)
  record()
  await handleTaskTool('task_start', { id: 'A' }, deps)
  record()
  expect(await handleTaskTool('task_done', {}, deps)).toBe('verify failed for A, attempts left: 2')
  record()
  expect(await handleTaskTool('task_done', {}, deps)).toBe('done A')
  record()
  expect(taskOf(current(), 'A')).toMatchObject({ phase: 'done', evidence: 'ran verify-a' })

  const revBeforeRefusal = current().rev
  expect(await handleTaskTool('task_start', { id: 'C' }, deps)).toBe('task C has unfinished deps')
  expect(current().rev).toBe(revBeforeRefusal)

  expect(await handleTaskTool('task_start', { id: 'B' }, deps)).toBe('started B')
  record()
  expect(await handleTaskTool('task_start', { id: 'C' }, deps)).toBe('task C has unfinished deps')
  expect(await handleTaskTool('task_done', {}, deps)).toBe('done B')
  record()
  expect(await handleTaskTool('task_start', { id: 'C' }, deps)).toBe('started C')
  record()

  expect(revs.every((rev, index) => index === 0 || rev > revs[index - 1])).toBe(true)
})

test('assertRev rejects a stale rev after a save', async () => {
  const { deps, current } = memoryDeps([])
  await handleTaskTool('task_plan', PLAN, deps)
  const staleRev = current().rev
  await handleTaskTool('task_start', { id: 'A' }, deps)
  expect(assertRev(current(), staleRev)).toBe(`stale state: expected rev ${staleRev}, found ${staleRev + 1}`)
  expect(assertRev(current(), current().rev)).toBeUndefined()
})
