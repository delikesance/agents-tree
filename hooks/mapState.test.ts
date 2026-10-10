import { expect, test } from 'claude-code/testing'

import { MAP_FILE, mapStatus } from './mapState'

const HEAD = 'b'.repeat(40)
const WRITTEN = 'a'.repeat(40)

const reading = (text?: string) => (path: string) => (path === MAP_FILE && text !== undefined ? Promise.resolve(text) : Promise.reject(new Error('absent')))
const diff = (stdout: string, exitCode = 0) => async () => ({ exitCode, stdout })

test('a missing map is missing', async () => {
  expect(await mapStatus(diff(''), reading(), HEAD)).toBe('missing')
})

test('a map without commit line is manual', async () => {
  expect(await mapStatus(diff('A\tx'), reading('# Hand written\n'), HEAD)).toBe('manual')
})

test('a map written at HEAD is fresh without asking git', async () => {
  const run = async () => {
    throw new Error('git must not run')
  }
  expect(await mapStatus(run, reading(`<!-- map: ${HEAD} -->\n`), HEAD)).toBe('fresh')
})

test('HEAD moved without added, deleted or renamed files stays fresh', async () => {
  expect(await mapStatus(diff(''), reading(`<!-- map: ${WRITTEN} -->\n`), HEAD)).toBe('fresh')
  expect(await mapStatus(diff(`A\t${MAP_FILE}\n`), reading(`<!-- map: ${WRITTEN} -->\n`), HEAD)).toBe('fresh')
})

test('an added, deleted or renamed file makes the map stale', async () => {
  const text = `<!-- map: ${WRITTEN} -->\n`
  expect(await mapStatus(diff('A\thooks/new.ts\n'), reading(text), HEAD)).toBe('stale')
  expect(await mapStatus(diff('D\told.ts\n'), reading(text), HEAD)).toBe('stale')
  expect(await mapStatus(diff('R100\told.ts\tnew.ts\n'), reading(text), HEAD)).toBe('stale')
})

test('a failing git diff makes the map stale', async () => {
  expect(await mapStatus(diff('', 128), reading(`<!-- map: ${WRITTEN} -->\n`), HEAD)).toBe('stale')
})
