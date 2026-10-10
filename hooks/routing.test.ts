import { expect, test } from 'claude-code/testing'

import { isWellBriefed } from './routing'

test('a brief with paths and a report limit skips the rewrite', () => {
  expect(isWellBriefed('Fix src/a.ts then report. Final report: 15 lines max.')).toBe(true)
})

test('a brief missing paths or a report limit is rewritten', () => {
  expect(isWellBriefed('Fix the sorting bug, report in 10 lines')).toBe(false)
  expect(isWellBriefed('Fix src/a.ts')).toBe(false)
})
