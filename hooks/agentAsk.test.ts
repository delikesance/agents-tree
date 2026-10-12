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
