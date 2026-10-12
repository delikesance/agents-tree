import { expect, test } from 'claude-code/testing'
import { createAgentAsk } from './agentAsk'

test('ask resolves with the answer settled for the spawned agent', async () => {
  const agentAsk = createAgentAsk()
  const pending = agentAsk.ask(async () => ({ agentId: 'a1' }), 'hello')
  await Promise.resolve()
  agentAsk.settle('a1', 'done')
  expect(await pending).toBe('done')
})

test('ask rejects when the spawn is refused', async () => {
  const agentAsk = createAgentAsk()
  await expect(agentAsk.ask(async () => ({ deny: 'no' }), 'hello')).rejects.toThrow('no')
})

test('a hand-back carrying the settled answer is a report even without the agent id', async () => {
  const ask = createAgentAsk()
  const pending = ask.ask(async () => ({ agentId: 'a1' }), 'p')
  await Promise.resolve()
  ask.settle('a1', 'draft skill body')
  await pending
  expect(ask.isReport('Message from @explorer: draft skill body')).toBe(true)
  expect(ask.isReport('unrelated')).toBe(false)
})
