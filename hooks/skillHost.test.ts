import { expect, test } from 'claude-code/testing'
import { skillDepsOf, type SkillHostSource } from './skillHost'

const hostWith = (data: Record<string, unknown>) =>
  ({ store: { get: async (key: string) => data[key], set: async (key: string, value: unknown) => void (data[key] = value) } }) as unknown as SkillHostSource

test('the skill ledger is kept per project', async () => {
  const data: Record<string, unknown> = {}
  const host = hostWith(data)
  await skillDepsOf(host, '/a').store.set('skills', { x: 1 })
  expect(await skillDepsOf(host, '/b').store.get('skills')).toBeUndefined()
  expect(await skillDepsOf(host, '/a').store.get('skills')).toEqual({ x: 1 })
  expect(Object.keys(data)).toEqual(['skills@/a'])
})
