import { expect, test } from 'claude-code/testing'

import { withMapPointer } from './routing'

test('the brief is prefixed with the map pointer only once, and only when a map exists', () => {
  const prefixed = withMapPointer('Fix src/a.ts', true)
  expect(prefixed).toBe('Read .claude/project-map.md first, do not sweep the repo.\n\nFix src/a.ts')
  expect(withMapPointer(prefixed, true)).toBe(prefixed)
  expect(withMapPointer('Read .claude/project-map.md then fix', true)).toBe('Read .claude/project-map.md then fix')
  expect(withMapPointer('Fix src/a.ts', false)).toBe('Fix src/a.ts')
})
