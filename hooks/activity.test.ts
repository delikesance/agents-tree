import { expect, test } from 'claude-code/testing'

import { hasUnfinished, hasUnfinishedTodos } from './activity'

test('main progress is kept only while items remain', () => {
  expect(hasUnfinished({ done: 1, total: 3 })).toBe(true)
  expect(hasUnfinished({ done: 3, total: 3 })).toBe(false)
  expect(hasUnfinished(undefined)).toBe(false)
})

test('main todos are kept while any item is not completed', () => {
  expect(hasUnfinishedTodos([{ content: 'a', status: 'completed' }, { content: 'b', status: 'pending' }])).toBe(true)
  expect(hasUnfinishedTodos([{ content: 'a', status: 'completed' }])).toBe(false)
  expect(hasUnfinishedTodos(undefined)).toBe(false)
})
