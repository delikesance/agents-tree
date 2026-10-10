export const BREAKER_FAILURES = 2
export const RETRY_OR_STOP = {
  retry: 'The failure looks transient or fixable by a small change; trying again can plausibly succeed.',
  stop: 'The same failure is repeating or is not fixable by retrying; further attempts are wasted.',
}
export const BREAKER_DENY = 'Échec répété de ce même appel: change d\'approche au lieu de réessayer.'
export const BREAKER_WINDOW_CALLS = 20
export const failures = new Map<string, string[]>()
const lastFailureCall = new Map<string, number>()
const history = { calls: 0, lastKey: '' }

export const startCall = () => {
  history.calls += 1
}

export const recordFailure = (key: string, output: string) => {
  if (history.calls - (lastFailureCall.get(key) ?? history.calls) > BREAKER_WINDOW_CALLS) failures.delete(key)
  lastFailureCall.set(key, history.calls)
  evictOldest(lastFailureCall)
  const attempts = [...(failures.get(key) ?? []), output]
  failures.set(key, attempts)
  evictOldest(failures)
  history.lastKey = key
  return attempts
}

export const recordSuccess = (key: string) => {
  if (history.lastKey !== key) failures.delete(key)
  history.lastKey = key
}
export const stopped = new Set<string>()
const BREAKER_MAX_KEYS = 500

export const evictOldest = (keys: { size: number; keys(): IterableIterator<string>; delete(key: string): boolean }) => {
  if (keys.size > BREAKER_MAX_KEYS) keys.delete(keys.keys().next().value as string)
}
