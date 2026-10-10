import { expect, test } from 'claude-code/testing'
import type { AgentInfo } from 'claude-code'

import { buildTree, flatten, rememberAgents } from './tree'

const agent = (id: string, status: AgentInfo['status']) => ({ id, status, type: 'explorer' }) as AgentInfo

test('an agent the host stops listing stays, marked completed', () => {
  rememberAgents([agent('a', 'running')])
  expect(rememberAgents([])).toEqual([agent('a', 'completed')])
})

test('a failed agent keeps its status when the host drops it', () => {
  rememberAgents([agent('b', 'failed')])
  expect(rememberAgents([]).find(({ id }) => id === 'b')?.status).toBe('failed')
})

test('a nested agent sits under its parent with its own type and a launcher', () => {
  const agents = [{ ...agent('p', 'running'), type: 'worker' }, { ...agent('c', 'running'), parentId: 'p' }] as AgentInfo[]
  const rows = flatten(buildTree(agents, {}, {}, {}, {}, {}, {}, false, 0))
  expect(rows.map(({ depth }) => depth)).toEqual([0, 1, 2])
  expect(rows[2].node.label).toContain('explorer')
  expect(rows[2].node.launchedBy).toBe(rows[1].node.label)
  expect(rows[2].last).toBe(true)
})
