import { expect, test } from 'claude-code/testing'

import { parseRemember } from './remember'

test('rejects empty key, text or tags', () => {
  expect('error' in parseRemember({ key: ' ', text: 'x', tags: ['a'] })).toBe(true)
  expect('error' in parseRemember({ key: 'k', text: '', tags: ['a'] })).toBe(true)
  expect('error' in parseRemember({ key: 'k', text: 'x', tags: [' '] })).toBe(true)
})

test('keeps valid input', () => {
  expect(parseRemember({ key: 'k', text: 'x', tags: ['a'] })).toEqual({ entry: { key: 'k', text: 'x', tags: ['a'] } })
})
