import { expect, test } from 'claude-code/testing'

import { isWellBriefed, withMapPointer } from './routing'

test('a brief with paths and a report limit skips the rewrite', () => {
  expect(isWellBriefed('Fix src/a.ts then report. Final report: 15 lines max.')).toBe(true)
})

test('a brief missing paths or a report limit is rewritten', () => {
  expect(isWellBriefed('Fix the sorting bug, report in 10 lines')).toBe(false)
  expect(isWellBriefed('Fix src/a.ts')).toBe(false)
})

test('the brief is prefixed with the map pointer only once, and only when a map exists', () => {
  const prefixed = withMapPointer('Fix src/a.ts', true)
  expect(prefixed).toBe('Read .claude/project-map.md first, do not sweep the repo.\n\nFix src/a.ts')
  expect(withMapPointer(prefixed, true)).toBe(prefixed)
  expect(withMapPointer('Read .claude/project-map.md then fix', true)).toBe('Read .claude/project-map.md then fix')
  expect(withMapPointer('Fix src/a.ts', false)).toBe('Fix src/a.ts')
})
