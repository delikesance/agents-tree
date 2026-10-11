import { expect, test } from 'claude-code/testing'
import type { AgentInfo } from 'claude-code'

import { cardHeight } from './cardHeight'
import { dismissAgent, isDismissed } from './select'
import { buildTree, flatten, rememberAgents, trackLifecycle, visibleAgents } from './tree'
import type { Node, Row, TrackedAgent } from './tree'

const agent = (id: string, status: string) => ({ id, status, type: 'explorer' }) as TrackedAgent

test('an agent the host stops listing stays one round, marked gone', () => {
  rememberAgents([agent('a', 'running')])
  expect(rememberAgents([])).toEqual([agent('a', 'gone')])
})

test('a failed agent keeps its status when the host drops it', () => {
  rememberAgents([agent('b', 'failed')])
  expect(rememberAgents([]).find(({ id }) => id === 'b')?.status).toBe('failed')
})

test('a nested agent sits under its parent with its own type and a launcher', () => {
  const agents = [{ ...agent('p', 'running'), type: 'worker' }, { ...agent('c', 'running'), parentId: 'p' }] as AgentInfo[]
  const rows = flatten(buildTree(agents, {}, {}, {}, {}, {}, {}, false, 0))
  expect(rows.map(({ depth }) => depth)).toEqual([0, 1, 2])
  expect(rows[2]?.node.label).toContain('explorer')
  expect(rows[2]?.node.launchedBy).toBe(rows[1]?.node.label)
  expect(rows[2]?.last).toBe(true)
})

test('workflow agents hang under one workflow node per run, summed and ordered', () => {
  const members = [{ id: 'w2', runId: 'wf_run', agentIndex: 2, type: 'worker', startedAt: 0, endedAt: 4 }, { id: 'w1', runId: 'wf_run', agentIndex: 1, type: 'worker', startedAt: 0 }]
  const usage = (cost: number) => ({ model: 'm', input: 1, output: 1, cacheRead: 0, cacheWrite: 0, cost, steps: 1 })
  const [group] = buildTree([], { w1: usage(1), w2: usage(2) }, {}, {}, {}, {}, {}, false, 10, members).children
  expect(group?.label).toBe('workflow · run')
  expect(group?.status).toBe('running')
  expect(group?.children).toEqual([])
  expect(group?.goal).toBe('1 terminés · 1 en cours')
  expect(group?.progress).toEqual({ done: 1, total: 2 })
  expect(group?.usage?.cost).toBe(3)
})

test('a workflow node is completed once every agent ended', () => {
  const members = [{ id: 'w1', runId: 'wf_x', agentIndex: 1, type: 'worker', startedAt: 0, endedAt: 3 }]
  expect(buildTree([], {}, {}, {}, {}, {}, {}, false, 10, members).children[0]?.status).toBe('completed')
})

const row = (node: Partial<Node> & { id: string }, depth: number): Row => ({ node: { label: node.id, status: 'completed', reveal: 1, children: [], ...node }, depth, last: true, ancestorsLast: [] })
const mainRow = row({ id: 'main', status: 'running' }, 0)

test('visibleAgents keeps live agents and their ancestors before finished ones', () => {
  const rows = [mainRow, row({ id: 'old' }, 1), row({ id: 'parent' }, 1), row({ id: 'child', status: 'running' }, 2)]
  const { shown, hidden } = visibleAgents(rows, 3 * 3)
  expect(shown.map(({ node }) => node.id)).toEqual(['main', 'parent', 'child'])
  expect(hidden).toBe(1)
})

test('visibleAgents fills spare space with finished agents, newest first', () => {
  const rows = [mainRow, row({ id: 'new' }, 1), row({ id: 'older' }, 1)]
  const { shown, hidden } = visibleAgents(rows, 2 * cardHeight(mainRow.node, true))
  expect(shown.map(({ node }) => node.id)).toEqual(['main', 'new'])
  expect(hidden).toBe(1)
})

test('visibleAgents shows everything when it fits', () => {
  const rows = [mainRow, row({ id: 'a' }, 1)]
  expect(visibleAgents(rows, 100)).toEqual({ shown: rows, hidden: 0 })
})

test('an agent that vanishes while live is gone, then purged with its entries', () => {
  rememberAgents([agent('g', 'running')])
  expect(rememberAgents([]).find(({ id }) => id === 'g')?.status).toBe('gone')
  expect(rememberAgents([]).find(({ id }) => id === 'g')).toBeUndefined()
})

test('a purged gone agent forgets its dismissal and lifecycle', () => {
  rememberAgents([agent('h', 'running')])
  trackLifecycle([agent('h', 'running')], 0)
  dismissAgent('h')
  rememberAgents([])
  rememberAgents([])
  expect(isDismissed('h')).toBe(false)
})

test('a child of main has no launcher', () => {
  const rows = flatten(buildTree([agent('m', 'running')], {}, {}, {}, {}, {}, {}, false, 0))
  expect(rows[1]?.node.launchedBy).toBeUndefined()
})

test('a launched workflow shows as a starting node before its first agent', () => {
  const [group] = buildTree([], {}, {}, {}, {}, {}, {}, false, 10, [], [4]).children
  expect(group?.label).toBe('workflow · démarrage')
  expect(group?.status).toBe('running')
})
