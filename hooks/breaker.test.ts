import { test, expect } from 'claude-code/testing'
import { BREAKER_WINDOW_CALLS, recordFailure, recordSuccess, resetBreaker, startCall, stopped } from './breaker'
import { contextNudge, resetNudge } from './compact'
test('success clears failures and stopped state', () => {
  resetBreaker()
  startCall(); expect(recordFailure('k', 'a').length).toBe(1)
  stopped.add('k')
  startCall(); recordSuccess('k')
  expect(stopped.has('k')).toBe(false)
  startCall(); expect(recordFailure('k', 'b').length).toBe(1)
})
test('resetBreaker clears everything', () => {
  startCall(); recordFailure('r', 'a'); stopped.add('r'); resetBreaker()
  expect(stopped.size).toBe(0)
  startCall(); expect(recordFailure('r', 'b').length).toBe(1)
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
test('nudge once', () => {
  resetNudge(); expect(contextNudge(1000)).toBeUndefined(); expect(contextNudge(130000)).toBeTruthy(); expect(contextNudge(130000)).toBeUndefined() })
test('nudge is per session and resettable', () => {
  expect(contextNudge(130000, 'a')).toBeTruthy()
  expect(contextNudge(130000, 'b')).toBeTruthy()
  expect(contextNudge(130000, 'a')).toBeUndefined()
  resetNudge('a')
  expect(contextNudge(130000, 'a')).toBeTruthy()
  expect(contextNudge(130000, 'b')).toBeUndefined()
})
