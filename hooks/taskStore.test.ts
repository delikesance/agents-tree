import { expect, test } from 'claude-code/testing'
import { createTaskToolDeps, loadTaskState, saveTaskState, STATE_FILE, type TaskIo } from './taskStore'
import type { TaskState } from './taskTypes'

const CWD = '/repo'
const STATE_PATH = `${CWD}/${STATE_FILE}`
const TEMP_PATH = `${STATE_PATH}.tmp`

const memoryIo = () => {
  const files = new Map<string, string>()
  const io: TaskIo = {
    read: async path => files.get(path),
    write: async (path, content) => {
      files.set(path, content)
    },
    rename: async (from, to) => {
      const content = files.get(from)
      if (content === undefined) throw new Error(`missing ${from}`)
      files.delete(from)
      files.set(to, content)
    },
  }
  return { io, files }
}

const stateAt = (rev: number): TaskState => ({ goal: 'ship', rev, tasks: [] })

test('loadTaskState returns undefined when the file is missing', async () => {
  const { io } = memoryIo()
  expect(await loadTaskState(io, CWD)).toBeUndefined()
})

test('loadTaskState returns undefined for malformed content', async () => {
  const { io, files } = memoryIo()
  files.set(STATE_PATH, '{not json')
  expect(await loadTaskState(io, CWD)).toBeUndefined()
  files.set(STATE_PATH, JSON.stringify({ rev: 1 }))
  expect(await loadTaskState(io, CWD)).toBeUndefined()
})

test('saveTaskState writes a temp file then renames it over the state file', async () => {
  const { io, files } = memoryIo()
  const renamed: string[] = []
  const recording: TaskIo = {
    ...io,
    rename: async (from, to) => {
      renamed.push(`${from}->${to}`)
      await io.rename(from, to)
    },
  }
  await saveTaskState(recording, CWD, stateAt(1))
  expect(renamed).toEqual([`${TEMP_PATH}->${STATE_PATH}`])
  expect(files.has(TEMP_PATH)).toBe(false)
  expect(await loadTaskState(io, CWD)).toEqual(stateAt(1))
})

test('saveTaskState writes when the disk rev is older than the new rev', async () => {
  const { io } = memoryIo()
  await saveTaskState(io, CWD, stateAt(1))
  await saveTaskState(io, CWD, stateAt(2))
  expect(await loadTaskState(io, CWD)).toEqual(stateAt(2))
})

test('saveTaskState refuses when the disk rev is not older than the new rev', async () => {
  const { io } = memoryIo()
  await saveTaskState(io, CWD, stateAt(3))
  await expect(saveTaskState(io, CWD, stateAt(3))).rejects.toThrow('stale task state')
  await expect(saveTaskState(io, CWD, stateAt(2))).rejects.toThrow('stale task state')
  expect(await loadTaskState(io, CWD)).toEqual(stateAt(3))
})

test('saveTaskState overwrites a malformed file', async () => {
  const { io, files } = memoryIo()
  files.set(STATE_PATH, 'garbage')
  await saveTaskState(io, CWD, stateAt(0))
  expect(await loadTaskState(io, CWD)).toEqual(stateAt(0))
})

test('createTaskToolDeps loads, saves and runs verify in the project cwd', async () => {
  const { io } = memoryIo()
  const calls: Array<[string, string]> = []
  const deps = createTaskToolDeps(io, CWD, async (cmd, cwd) => {
    calls.push([cmd, cwd])
    return { code: 0, output: 'ok' }
  })
  await deps.save(stateAt(1))
  expect(await deps.load()).toEqual(stateAt(1))
  expect(await deps.runVerify('bun test')).toEqual({ code: 0, output: 'ok' })
  expect(calls).toEqual([['bun test', CWD]])
})
