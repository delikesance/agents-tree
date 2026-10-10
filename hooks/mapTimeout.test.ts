import { expect, test } from 'claude-code/testing'

import { writeMap } from './mapGenerate'
import type { MapIo } from './mapGenerate'
import { MAP_FILE } from './mapState'

const SUMMARY_TIMEOUT_MS = 20_000

test('a model call that never settles times out and the map is written once, without summaries', async () => {
  const written: Array<[string, string]> = []
  const waits: number[] = []
  const realSetTimeout = globalThis.setTimeout
  globalThis.setTimeout = ((callback: () => void, ms?: number) => {
    waits.push(ms ?? 0)
    return realSetTimeout(callback, 0)
  }) as typeof setTimeout
  const io: MapIo = {
    run: async () => ({ exitCode: 0, stdout: 'src/main.ts\0src/a.ts\0' }),
    exists: async () => false,
    read: async () => Promise.reject(new Error('absent')),
    write: async (path, text) => void written.push([path, text]),
    complete: () => new Promise<string>(() => undefined),
  }
  try {
    await writeMap(io, 'd'.repeat(40))
  } finally {
    globalThis.setTimeout = realSetTimeout
  }
  expect(waits).toContain(SUMMARY_TIMEOUT_MS)
  expect(written).toHaveLength(1)
  expect(written[0]?.[0]).toBe(MAP_FILE)
  expect(written[0]?.[1]).toContain('- src/ -> 2 files (.ts)\n')
})

test('nothing is written when listing files fails, so no half-written map is left', async () => {
  const written: string[] = []
  const io: MapIo = {
    run: async () => ({ exitCode: 1, stdout: '' }),
    exists: async () => false,
    read: async () => Promise.reject(new Error('absent')),
    write: async (_path, text) => void written.push(text),
    complete: async () => 'x',
  }
  await expect(writeMap(io, 'd'.repeat(40))).rejects.toThrow()
  expect(written).toEqual([])
})
