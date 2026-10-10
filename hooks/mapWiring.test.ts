import { expect, test } from 'claude-code/testing'

import { pointAtMap, startMapUpkeep } from './mapWiring'
import { MAP_FILE } from './mapState'

type Host = Parameters<typeof startMapUpkeep>[0]

const hostWith = (overrides: Partial<Host['session']> & { exists?: (path: string) => Promise<boolean> } = {}) => {
  const calls: string[] = []
  const host: Host = {
    process: { run: async () => ({ exitCode: 128, stdout: '' }) },
    fs: { read: async () => '', write: async () => undefined, exists: overrides.exists ?? (async () => true) },
    model: { complete: async () => ({ isAnswered: false }) },
    session: { id: overrides.id ?? (async () => (calls.push('session'), 'wiring')) },
  }
  return { host, calls }
}

test('map upkeep starts for a prompt and returns before it settles', () => {
  const { host, calls } = hostWith()
  expect(startMapUpkeep(host, '/p', false)).toBeUndefined()
  expect(calls).toEqual(['session'])
})

test('map upkeep is skipped for commands', () => {
  const { host, calls } = hostWith()
  startMapUpkeep(host, '/p', true)
  expect(calls).toEqual([])
})

test('a failing upkeep never rejects into the hook', async () => {
  const unhandled: unknown[] = []
  const record = (reason: unknown) => void unhandled.push(reason)
  process.on('unhandledRejection', record)
  const { host } = hostWith({ id: async () => Promise.reject(new Error('no session')) })
  startMapUpkeep(host, '/p', false)
  await new Promise(resolve => setTimeout(resolve, 10))
  process.off('unhandledRejection', record)
  expect(unhandled).toEqual([])
})

test('the pointer is added when the map exists in the project', async () => {
  const checked: string[] = []
  const { host } = hostWith({ exists: async path => (checked.push(path), true) })
  const prompt = await pointAtMap(host, '/p', 'do it')
  expect(checked).toEqual([`/p/${MAP_FILE}`])
  expect(prompt).toContain('do it')
  expect(prompt).toContain(MAP_FILE)
})

test('the prompt is untouched when the map is missing or the check fails', async () => {
  expect(await pointAtMap(hostWith({ exists: async () => false }).host, '/p', 'do it')).toBe('do it')
  expect(await pointAtMap(hostWith({ exists: async () => Promise.reject(new Error('fs')) }).host, '/p', 'do it')).toBe('do it')
})
