import { expect, test } from 'claude-code/testing'

import { groupFiles } from './mapGroups'
import { manualBlock } from './mapManual'
import { buildMap } from './mapSkeleton'
import type { MapInput } from './mapSkeleton'
import { renderGroups } from './mapRender'
import { evenSample, parseSummaries } from './mapSummaries'

const HEAD = 'c'.repeat(40)
const FILES = ['README.md', 'hooks/register.tsx', 'hooks/a.ts', 'hooks/a.test.ts', 'src/main.rs', 'crates/x/src/main.rs', 'crates/x/src/lib.rs', 'crates/y/src/main.rs']

const build = (overrides: Partial<MapInput> = {}, files = FILES) =>
  buildMap({ head: HEAD, files, groups: groupFiles(files), summaries: new Map(), symbols: new Map(), manual: manualBlock(), ...overrides })

test('line 1 is the exact commit marker', () => {
  expect(build().split('\n')[0]).toBe(`<!-- map: ${HEAD} -->`)
})

test('groups show code and test counts, symbols and purposes', () => {
  const groups = groupFiles(FILES)
  const summaries = parseSummaries('hooks/: plugin code\nghost: nothing\nnot a line', groups)
  expect([...summaries.keys()]).toEqual(['hooks/'])
  const map = build({ summaries, symbols: new Map([['hooks/a.ts', ['alpha', 'beta']]]) })
  expect(map).toContain('- hooks/ (2 files + 1 test): plugin code -> a.ts(alpha, beta)')
  expect(map).toContain('- crates/x/ (2 files)')
})

test('commands and deep entry points are listed, docs are not', () => {
  const map = build({ testCommand: 'bun test' })
  expect(map).toContain('- Test: `bun test`')
  expect(map).toContain('## Entry points\n- src/main.rs\n- crates/x/src/main.rs\n- crates/y/src/main.rs\n- hooks/register.tsx\n- crates/x/src/lib.rs')
  expect(map).not.toContain('## Docs')
})

test('a hand-written block is carried over verbatim, or an empty Pitfalls heading is offered', () => {
  const written = build({ manual: manualBlock('<!-- map: x -->\n# Project map\n\n<!-- manual -->\n## Pitfalls\n- never run X\n') })
  expect(written.endsWith('\n<!-- manual -->\n## Pitfalls\n- never run X\n')).toBe(true)
  expect(build().endsWith('\n\n<!-- manual -->\n## Pitfalls\n')).toBe(true)
})

test('the generated part stays within the line and size caps, without losing group lines', () => {
  const many = Array.from({ length: 500 }, (_, i) => `dir${i}/main.ts`)
  const symbols = new Map(many.map(file => [file, Array.from({ length: 6 }, (_, i) => `someLongSymbolName${i}`)]))
  const map = build({ symbols }, many)
  const generated = map.slice(0, map.indexOf('<!-- manual -->'))
  expect(generated.trimEnd().split('\n').length).toBeLessThanOrEqual(60)
  expect(generated.length).toBeLessThanOrEqual(8_000)
  expect(map).toContain('{dir0..dir')
})

test('small groups get symbols while huge ones stay at file names', () => {
  const files = Array.from({ length: 30 }, (_, group) => Array.from({ length: group + 1 }, (_, i) => `d${group}/f${i}.ts`)).flat()
  const names = Array.from({ length: 6 }, (_, i) => `veryLongExportedSymbolName${i}`)
  const symbols = new Map(files.map(file => [file, names]))
  const lines = build({ symbols }, files).split('\n')
  expect(lines.find(line => line.startsWith('- d0/'))).toContain('f0.ts(veryLongExportedSymbolName0, +5)')
  const biggest = lines.find(line => line.startsWith('- d29/')) ?? ''
  expect(biggest).toContain('f0.ts; f1.ts')
  expect(biggest).not.toContain('veryLongExportedSymbolName')
})

const TWELVE_FILES = Array.from({ length: 12 }, (_, i) => `lib/f${i}.ts`)
const twelveFileLine = (charBudget: number) => {
  const symbols = new Map(TWELVE_FILES.map(file => [file, ['x']]))
  return renderGroups(groupFiles(TWELVE_FILES), new Map(), symbols, charBudget).join('\n')
}

test('a group within a generous budget lists every file', () => {
  const line = twelveFileLine(8_000)
  expect(line).toContain('f11.ts(x)')
  expect(line).not.toContain('+')
})

test('a group in a tight budget stays capped at eight files', () => {
  const line = twelveFileLine(130)
  expect(line).toContain('f7.ts(x); +4')
  expect(line).not.toContain('f8.ts')
})

test('an even sample spans the list and leaves short lists unchanged', () => {
  expect(evenSample(['a', 'b'], 3)).toEqual(['a', 'b'])
  expect(evenSample(Array.from({ length: 37 }, (_, i) => i), 8)).toEqual([0, 4, 9, 13, 18, 23, 27, 32])
})

test('counts use singular and plural', () => {
  const map = build({}, ['a/x.ts', 'a/x.test.ts', 'a/y.test.ts'])
  expect(map).toContain('(1 file + 2 tests)')
})
