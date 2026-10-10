import { expect, test } from 'claude-code/testing'

import { buildMap, parseSummaries, topDirectories } from './mapSkeleton'

const HEAD = 'c'.repeat(40)
const FILES = ['README.md', 'hooks/register.tsx', 'hooks/a.ts', 'hooks/b.ts', 'docs/CLAUDE.md', 'src/main.rs']

test('line 1 is the exact commit marker', () => {
  expect(buildMap({ head: HEAD, files: FILES, summaries: new Map() }).split('\n')[0]).toBe(`<!-- map: ${HEAD} -->`)
})

test('layout counts files and lists dominant extensions, biggest directory first', () => {
  const map = buildMap({ head: HEAD, files: FILES, summaries: new Map() })
  expect(map).toContain('- hooks/ -> 3 files (.ts, .tsx)')
  expect(map).toContain('- root files -> 1 files (.md)')
  expect(topDirectories(FILES)[0]?.name).toBe('hooks')
})

test('commands, entry points and docs are listed', () => {
  const map = buildMap({ head: HEAD, files: FILES, testCommand: 'bun test', summaries: new Map() })
  expect(map).toContain('- Test: `bun test`')
  expect(map).toContain('## Entry points\n- hooks/register.tsx\n- src/main.rs')
  expect(map).toContain('## Docs\n- README.md\n- docs/CLAUDE.md')
})

test('summaries land on their directory and unknown ones are dropped', () => {
  const dirs = topDirectories(FILES)
  const summaries = parseSummaries('hooks: plugin code\nghost: nothing\nnot a line', dirs)
  expect([...summaries.keys()]).toEqual(['hooks'])
  expect(buildMap({ head: HEAD, files: FILES, summaries })).toContain('- hooks/ -> 3 files (.ts, .tsx): plugin code')
})

test('the map stays within the line cap', () => {
  const many = Array.from({ length: 500 }, (_, i) => `dir${i}/main.ts`)
  expect(buildMap({ head: HEAD, files: many, summaries: new Map() }).split('\n').length).toBeLessThanOrEqual(100)
})
