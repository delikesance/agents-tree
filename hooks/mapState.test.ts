import { expect, test } from 'claude-code/testing'

import { MAP_FILE, mapStatus } from './mapState'
import type { MapReader, Run } from './mapState'

const HEAD = 'b'.repeat(40)
const WRITTEN = 'a'.repeat(40)

const reader = (run: Run, text?: string): MapReader => ({
  run,
  exists: async () => text !== undefined,
  read: async () => text ?? Promise.reject(new Error('absent')),
})
const diff = (stdout: string, exitCode = 0) => async () => ({ exitCode, stdout })

test('a missing map is missing', async () => {
  expect(await mapStatus(reader(diff('')), HEAD)).toBe('missing')
})

test('a map without commit line is manual', async () => {
  expect(await mapStatus(reader(diff('A\tx'), '# Hand written\n'), HEAD)).toBe('manual')
})

test('an existing map that cannot be read is manual', async () => {
  const unreadable: MapReader = { ...reader(diff('A\tx')), exists: async () => true }
  expect(await mapStatus(unreadable, HEAD)).toBe('manual')
})

test('a map written at HEAD is fresh without asking git', async () => {
  const run = async () => {
    throw new Error('git must not run')
  }
  expect(await mapStatus(reader(run, `<!-- map: ${HEAD} -->\n`), HEAD)).toBe('fresh')
})

test('HEAD moved without added, deleted or renamed files stays fresh', async () => {
  expect(await mapStatus(reader(diff(''), `<!-- map: ${WRITTEN} -->\n`), HEAD)).toBe('fresh')
  expect(await mapStatus(reader(diff(`A\t${MAP_FILE}\n`), `<!-- map: ${WRITTEN} -->\n`), HEAD)).toBe('fresh')
})

test('an added, deleted or renamed file makes the map stale', async () => {
  const text = `<!-- map: ${WRITTEN} -->\n`
  expect(await mapStatus(reader(diff('A\thooks/new.ts\n'), text), HEAD)).toBe('stale')
  expect(await mapStatus(reader(diff('D\told.ts\n'), text), HEAD)).toBe('stale')
  expect(await mapStatus(reader(diff('R100\told.ts\tnew.ts\n'), text), HEAD)).toBe('stale')
})

test('a failing git diff makes the map stale', async () => {
  expect(await mapStatus(reader(diff('', 128), `<!-- map: ${WRITTEN} -->\n`), HEAD)).toBe('stale')
})
