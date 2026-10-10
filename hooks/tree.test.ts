import { expect, test } from 'claude-code/testing'
import type { AgentInfo } from 'claude-code'

import { rememberAgents } from './tree'

const agent = (id: string, status: AgentInfo['status']) => ({ id, status, type: 'explorer' }) as AgentInfo

test('an agent the host stops listing stays, marked completed', () => {
  rememberAgents([agent('a', 'running')])
  expect(rememberAgents([])).toEqual([agent('a', 'completed')])
})

test('a failed agent keeps its status when the host drops it', () => {
  rememberAgents([agent('b', 'failed')])
  expect(rememberAgents([]).find(({ id }) => id === 'b')?.status).toBe('failed')
})
