import { test, expect } from 'claude-code/testing'
import { BREAKER_WINDOW_CALLS, recordFailure, recordSuccess, startCall } from './breaker'
import { contextNudge } from './compact'
test('same-key success keeps count', () => {
  startCall(); expect(recordFailure('k', 'a').length).toBe(1)
  startCall(); recordSuccess('k')
  startCall(); expect(recordFailure('k', 'b').length).toBe(2)
})
test('other key success resets', () => {
  startCall(); recordFailure('x', 'a'); startCall(); recordSuccess('y'); startCall(); recordSuccess('x')
  startCall(); expect(recordFailure('x', 'b').length).toBe(1)
})
test('window expires', () => {
  startCall(); recordFailure('w', 'a')
  for (let i = 0; i <= BREAKER_WINDOW_CALLS; i++) startCall()
  expect(recordFailure('w', 'b').length).toBe(1)
})
test('nudge once', () => { expect(contextNudge(1000)).toBeUndefined(); expect(contextNudge(130000)).toBeTruthy(); expect(contextNudge(130000)).toBeUndefined() })
