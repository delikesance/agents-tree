import { expect, test } from 'claude-code/testing'
import { TASK_TOOLS, handleTaskTool, type TaskToolDeps } from './taskTools'
import type { TaskState } from './taskTypes'

function memoryDeps(verify = { code: 0, output: 'ok' }) {
  let stored: TaskState | undefined
  const deps: TaskToolDeps = {
    load: async () => stored,
    save: async state => {
      stored = state
    },
    runVerify: async () => verify,
  }
  return { deps, current: () => stored }
}

const plan = {
  goal: 'ship',
  tasks: [
    { id: 't1', title: 'one', verify: 'true' },
    { id: 't2', title: 'two', deps: ['t1'], verify: 'true' },
  ],
}

async function planned(verify?: { code: number; output: string }) {
  const memory = memoryDeps(verify)
  await handleTaskTool('task_plan', plan, memory.deps)
  return memory
}

test('exposes the six task tools', () => {
  expect(TASK_TOOLS.map(tool => tool.name)).toEqual([
    'task_plan',
    'task_start',
    'task_done',
    'task_block',
    'task_unblock',
    'task_add',
  ])
})

test('asks for a plan before any other task tool', async () => {
  const { deps } = memoryDeps()
  expect(await handleTaskTool('task_start', { id: 't1' }, deps)).toBe('no task plan: call task_plan first')
})

test('plan saves tasks as pending with zero attempts', async () => {
  const { current } = await planned()
  expect(current()?.tasks.map(task => task.phase)).toEqual(['pending', 'pending'])
  expect(current()?.tasks[0].attempts).toBe(0)
})

test('plan rejects a dependency cycle without saving', async () => {
  const memory = memoryDeps()
  const cyclic = {
    goal: 'g',
    tasks: [
      { id: 'a', title: 'a', deps: ['b'], verify: 'true' },
      { id: 'b', title: 'b', deps: ['a'], verify: 'true' },
    ],
  }
  expect(await handleTaskTool('task_plan', cyclic, memory.deps)).toBe('dependency cycle at a')
  expect(memory.current()).toBe(undefined)
})

test('start activates a pending task with finished deps', async () => {
  const { deps } = await planned()
  expect(await handleTaskTool('task_start', { id: 't2' }, deps)).toBe('task t2 has unfinished deps')
  expect(await handleTaskTool('task_start', { id: 't1' }, deps)).toBe('started t1')
})

test('done with no active task reports it', async () => {
  const { deps } = await planned()
  expect(await handleTaskTool('task_done', {}, deps)).toBe('no active task')
})

test('done finishes the active task when verify exits 0', async () => {
  const { deps, current } = await planned({ code: 0, output: 'all green' })
  await handleTaskTool('task_start', { id: 't1' }, deps)
  expect(await handleTaskTool('task_done', {}, deps)).toBe('done t1')
  const done = current()?.tasks.find(task => task.id === 't1')
  expect(done?.phase).toBe('done')
  expect(done?.evidence).toBe('all green')
})

test('done keeps only the last 2000 characters of verify output as evidence', async () => {
  const output = `head${'x'.repeat(3000)}tail`
  const { deps, current } = await planned({ code: 0, output })
  await handleTaskTool('task_start', { id: 't1' }, deps)
  await handleTaskTool('task_done', {}, deps)
  expect(current()?.tasks[0].evidence).toBe(output.slice(-2000))
})

test('done counts failed verifies and blocks after the last attempt', async () => {
  const verify = { code: 1, output: 'boom' }
  const { deps, current } = await planned(verify)
  await handleTaskTool('task_start', { id: 't1' }, deps)
  expect(await handleTaskTool('task_done', {}, deps)).toBe('verify failed for t1, attempts left: 2')
  expect(await handleTaskTool('task_done', {}, deps)).toBe('verify failed for t1, attempts left: 1')
  expect(await handleTaskTool('task_done', {}, deps)).toBe('blocked: ask the user (t1 failed 3 times)')
  expect(current()?.tasks[0].phase).toBe('blocked')
})

test('block marks a task blocked with its reason', async () => {
  const { deps, current } = await planned()
  expect(await handleTaskTool('task_block', { id: 't1', reason: 'needs key' }, deps)).toBe('blocked t1')
  expect(current()?.tasks[0]).toMatchObject({ phase: 'blocked', evidence: 'needs key' })
})

test('unblock returns a blocked task to pending and rejects other phases', async () => {
  const { deps, current } = await planned()
  expect(await handleTaskTool('task_unblock', { id: 't1' }, deps)).toBe('task t1 is pending, not blocked')
  await handleTaskTool('task_block', { id: 't1', reason: 'needs key' }, deps)
  expect(await handleTaskTool('task_unblock', { id: 't1' }, deps)).toBe('unblocked t1')
  expect(current()?.tasks[0]).toMatchObject({ phase: 'pending', attempts: 0, evidence: 'needs key' })
})

test('add appends a pending task to the plan', async () => {
  const { deps, current } = await planned()
  const added = { id: 't3', title: 'three', verify: 'true', deps: ['t1'] }
  expect(await handleTaskTool('task_add', added, deps)).toBe('added t3')
  expect(current()?.tasks[2]).toMatchObject({ id: 't3', phase: 'pending', attempts: 0, files: [] })
})

test('add rejects a duplicate id', async () => {
  const { deps } = await planned()
  expect(await handleTaskTool('task_add', { id: 't1', title: 'dup', verify: 'true' }, deps)).toBe(
    'duplicate task id: t1',
  )
})
