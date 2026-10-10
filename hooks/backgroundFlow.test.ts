import { expect, test } from 'claude-code/testing'

import { stashFlow, takeFlow } from './backgroundFlow'

test('a stashed flow is injected once', () => {
  stashFlow('flow')
  expect(takeFlow()).toBe('flow')
  expect(takeFlow()).toBeUndefined()
})
