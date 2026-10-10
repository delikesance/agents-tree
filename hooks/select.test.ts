import { expect, test } from 'claude-code/testing'

import { dismissAgent, isDismissed, selectAgent, selectedAgent } from './select'

test('selection is set then cleared', () => {
  selectAgent('a1')
  expect(selectedAgent()).toBe('a1')
  selectAgent(undefined)
  expect(selectedAgent()).toBeUndefined()
})

test('dismissed agents stay dismissed', () => {
  expect(isDismissed('a2')).toBe(false)
  dismissAgent('a2')
  expect(isDismissed('a2')).toBe(true)
})
