import { expect, test } from 'claude-code/testing'

import { groupFiles } from './mapGroups'
import { manualBlock } from './mapManual'
import { buildMap } from './mapSkeleton'
import type { MapInput } from './mapSkeleton'
import { parseSummaries } from './mapSummaries'

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
  expect(map).toContain('## Entry points\n- hooks/register.tsx\n- src/main.rs\n- crates/x/src/main.rs\n- crates/x/src/lib.rs\n- crates/y/src/main.rs')
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

test('symbols shrink uniformly before they are dropped, so big groups still name their files', () => {
  const files = [...Array.from({ length: 60 }, (_, i) => `big/f${i}.ts`), 'small/a.ts']
  const names = Array.from({ length: 6 }, (_, i) => `veryLongExportedSymbolName${i}`)
  const symbols = new Map(files.map(file => [file, names]))
  const map = build({ symbols }, files)
  expect(map).toContain('- small/ (1 file) -> a.ts(veryLongExportedSymbolName0')
  expect(map).toContain('f59.ts')
  expect(map).not.toContain('veryLongExportedSymbolName3')
})

test('counts use singular and plural', () => {
  const map = build({}, ['a/x.ts', 'a/x.test.ts', 'a/y.test.ts'])
  expect(map).toContain('(1 file + 2 tests)')
})
