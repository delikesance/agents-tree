import { expect, test } from 'claude-code/testing'
import { addTask, assertRev, blockTask, failTask, finishTask, nextRunnable, planTasks, startTask, type TaskResult, unblockTask } from './taskstate'
import { MAX_TASK_ATTEMPTS, type Task, type TaskState } from './taskTypes'

const task = (id: string, deps: string[] = [], overrides: Partial<Task> = {}): Task => ({
  id,
  title: id,
  phase: 'pending',
  deps,
  files: [],
  verify: '',
  attempts: 0,
  ...overrides,
})

const expectState = (result: TaskResult): TaskState => {
  if ('error' in result) throw new Error(result.error)
  return result
}

const errorOf = (result: TaskResult): string | undefined => ('error' in result ? result.error : undefined)

const planned = (...tasks: Task[]): TaskState => expectState(planTasks('goal', tasks))

const phaseOf = (state: TaskState, id: string) => state.tasks.find(item => item.id === id)?.phase

const chain = planned(task('a'), task('b', ['a']))

test('planTasks accepts a valid DAG at rev 0', () => {
  const state = planned(task('a'), task('b', ['a']))
  expect(state.goal).toBe('goal')
  expect(state.rev).toBe(0)
  expect(state.tasks.map(item => item.id)).toEqual(['a', 'b'])
})

test('planTasks rejects duplicate ids', () => {
  expect(errorOf(planTasks('g', [task('a'), task('a')]))).toBe('duplicate task id: a')
})

test('planTasks rejects unknown deps', () => {
  expect(errorOf(planTasks('g', [task('a', ['zz'])]))).toBe('unknown dep zz for task a')
})

test('planTasks rejects tasks that are not pending with 0 attempts', () => {
  const message = 'task a must start pending with 0 attempts'
  expect(errorOf(planTasks('g', [task('a', [], { phase: 'active' })]))).toBe(message)
  expect(errorOf(planTasks('g', [task('a', [], { phase: 'done' })]))).toBe(message)
  expect(errorOf(planTasks('g', [task('a', [], { attempts: 1 })]))).toBe(message)
})

test('planTasks rejects self loops and longer cycles', () => {
  expect(errorOf(planTasks('g', [task('a', ['a'])]))).toBe('dependency cycle at a')
  const ring = [task('a', ['c']), task('b', ['a']), task('c', ['b'])]
  expect(errorOf(planTasks('g', ring))).toBe('dependency cycle at a')
})

test('planTasks copies tasks so the input is not shared', () => {
  const input = [task('a')]
  const state = planned(...input)
  expect(state.tasks[0]).not.toBe(input[0])
})

test('startTask moves a pending task with done deps to active and bumps rev', () => {
  const done = expectState(finishTask(expectState(startTask(chain, 'a')), 'a', 'ok'))
  const started = expectState(startTask(done, 'b'))
  expect(phaseOf(started, 'b')).toBe('active')
  expect(started.rev).toBe(done.rev + 1)
})

test('startTask does not mutate the previous state', () => {
  const started = expectState(startTask(chain, 'a'))
  expect(phaseOf(chain, 'a')).toBe('pending')
  expect(chain.rev).toBe(0)
  expect(started.rev).toBe(1)
})

test('startTask rejects unknown ids', () => {
  expect(errorOf(startTask(chain, 'zz'))).toBe('unknown task zz')
})

test('startTask rejects tasks that are not pending', () => {
  const active = expectState(startTask(chain, 'a'))
  expect(errorOf(startTask(active, 'a'))).toBe('task a is active, not pending')
  const done = expectState(finishTask(active, 'a', 'ok'))
  expect(errorOf(startTask(done, 'a'))).toBe('task a is done, not pending')
})

test('startTask rejects tasks whose deps are not done', () => {
  expect(errorOf(startTask(chain, 'b'))).toBe('task b has unfinished deps')
})

test('startTask rejects a second active task', () => {
  const state = planned(task('a'), task('b'))
  const active = expectState(startTask(state, 'a'))
  expect(errorOf(startTask(active, 'b'))).toBe('task a is already active')
})

test('finishTask marks an active task done with evidence', () => {
  const active = expectState(startTask(chain, 'a'))
  const finished = expectState(finishTask(active, 'a', 'tests green'))
  const record = finished.tasks.find(item => item.id === 'a')
  expect(record?.phase).toBe('done')
  expect(record?.evidence).toBe('tests green')
  expect(finished.rev).toBe(active.rev + 1)
})

test('finishTask rejects non-active and unknown tasks', () => {
  expect(errorOf(finishTask(chain, 'a', 'x'))).toBe('task a is pending, not active')
  expect(errorOf(finishTask(chain, 'zz', 'x'))).toBe('unknown task zz')
})

test('failTask keeps the task active and records output while attempts remain', () => {
  const active = expectState(startTask(chain, 'a'))
  const failed = expectState(failTask(active, 'a', 'boom'))
  const record = failed.tasks.find(item => item.id === 'a')
  expect(record?.phase).toBe('active')
  expect(record?.attempts).toBe(1)
  expect(record?.evidence).toBe('boom')
})

test('failTask blocks the task once MAX_TASK_ATTEMPTS is reached', () => {
  let state = expectState(startTask(chain, 'a'))
  for (let attempt = 1; attempt < MAX_TASK_ATTEMPTS; attempt++) {
    state = expectState(failTask(state, 'a', `fail ${attempt}`))
    expect(phaseOf(state, 'a')).toBe('active')
  }
  state = expectState(failTask(state, 'a', 'final'))
  expect(phaseOf(state, 'a')).toBe('blocked')
  expect(state.tasks[0].attempts).toBe(MAX_TASK_ATTEMPTS)
})

test('failTask rejects non-active and unknown tasks', () => {
  expect(errorOf(failTask(chain, 'a', 'x'))).toBe('task a is pending, not active')
  expect(errorOf(failTask(chain, 'zz', 'x'))).toBe('unknown task zz')
})

test('blockTask blocks pending and active tasks with the reason as evidence', () => {
  const blockedPending = expectState(blockTask(chain, 'b', 'needs input'))
  expect(phaseOf(blockedPending, 'b')).toBe('blocked')
  expect(blockedPending.tasks[1].evidence).toBe('needs input')
  const blockedActive = expectState(blockTask(expectState(startTask(chain, 'a')), 'a', 'stuck'))
  expect(phaseOf(blockedActive, 'a')).toBe('blocked')
})

test('blockTask rejects done, already blocked and unknown tasks', () => {
  const done = expectState(finishTask(expectState(startTask(chain, 'a')), 'a', 'ok'))
  expect(errorOf(blockTask(done, 'a', 'x'))).toBe('task a is already done')
  const blocked = expectState(blockTask(chain, 'a', 'x'))
  expect(errorOf(blockTask(blocked, 'a', 'x'))).toBe('task a is already blocked')
  expect(errorOf(blockTask(chain, 'zz', 'x'))).toBe('unknown task zz')
})

test('unblockTask returns a blocked task to pending with attempts reset and evidence kept', () => {
  let state = expectState(startTask(chain, 'a'))
  state = expectState(failTask(state, 'a', 'boom'))
  state = expectState(blockTask(state, 'a', 'needs key'))
  const unblocked = expectState(unblockTask(state, 'a'))
  expect(unblocked.tasks[0]).toMatchObject({ phase: 'pending', attempts: 0, evidence: 'needs key' })
  expect(unblocked.rev).toBe(state.rev + 1)
})

test('unblockTask rejects tasks that are not blocked and unknown tasks', () => {
  expect(errorOf(unblockTask(chain, 'a'))).toBe('task a is pending, not blocked')
  expect(errorOf(unblockTask(chain, 'zz'))).toBe('unknown task zz')
})

test('addTask appends a fresh task with valid deps', () => {
  const added = expectState(addTask(chain, task('c', ['b'])))
  expect(added.tasks.map(item => item.id)).toEqual(['a', 'b', 'c'])
  expect(added.rev).toBe(chain.rev + 1)
})

test('addTask keeps a graph that already has progress valid', () => {
  const active = expectState(startTask(chain, 'a'))
  expect(errorOf(addTask(active, task('c', ['a'])))).toBeUndefined()
})

test('addTask rejects duplicate ids', () => {
  expect(errorOf(addTask(chain, task('a')))).toBe('duplicate task id: a')
})

test('addTask rejects a task that is not fresh', () => {
  expect(errorOf(addTask(chain, task('c', [], { phase: 'done' })))).toBe('task c must start pending with 0 attempts')
})

test('addTask rejects unknown deps', () => {
  expect(errorOf(addTask(chain, task('c', ['zz'])))).toBe('unknown dep zz for task c')
})

test('addTask rejects a self loop', () => {
  expect(errorOf(addTask(chain, task('c', ['c'])))).toBe('dependency cycle at c')
})

test('nextRunnable lists pending tasks whose deps are done', () => {
  expect(nextRunnable(chain).map(item => item.id)).toEqual(['a'])
  const done = expectState(finishTask(expectState(startTask(chain, 'a')), 'a', 'ok'))
  expect(nextRunnable(done).map(item => item.id)).toEqual(['b'])
})

test('nextRunnable excludes active and blocked tasks', () => {
  const active = expectState(startTask(planned(task('a'), task('b')), 'a'))
  expect(nextRunnable(active).map(item => item.id)).toEqual(['b'])
  const blocked = expectState(blockTask(planned(task('a'), task('b')), 'a', 'x'))
  expect(nextRunnable(blocked).map(item => item.id)).toEqual(['b'])
})

test('assertRev returns undefined on match and an error otherwise', () => {
  expect(assertRev(chain, 0)).toBeUndefined()
  expect(assertRev(chain, 1)).toBe('stale state: expected rev 1, found 0')
})
