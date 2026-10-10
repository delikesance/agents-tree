import { agentName } from './names'
import { isDismissed } from './select'
import type { AgentInfo } from 'claude-code'
import type { AgentUsage, GoalProgress } from '../types'
import type { Todo } from './activity'

export const MAIN = 'main'
export const isLive = (status: string) => status === 'running' || status === 'waiting' || status === 'pending'

export type Node = { id: string; label: string; status: string; usage?: AgentUsage; activity?: string; model?: string; goal?: string; progress?: GoalProgress; todos?: Todo[]; elapsed?: number; reveal: number; finished?: boolean; launchedBy?: string; children: Node[] }

type Lifecycle = { startedAt: number; endedAt?: number }
const lifecycle = new Map<string, Lifecycle>()

export const trackLifecycle = (agents: readonly AgentInfo[], now: number) => {
  if (!lifecycle.has(MAIN)) lifecycle.set(MAIN, { startedAt: now })
  agents.forEach(({ id, status }) => {
    const record = lifecycle.get(id) ?? { startedAt: now, endedAt: isLive(status) ? undefined : 0 }
    if (!isLive(status) && record.endedAt === undefined) record.endedAt = now
    lifecycle.set(id, record)
  })
}

const seen = new Map<string, AgentInfo>()

export const rememberAgents = (agents: readonly AgentInfo[]): AgentInfo[] => {
  const current = new Set(agents.map(({ id }) => id))
  agents.forEach(agent => seen.set(agent.id, agent))
  seen.forEach((agent, id) => {
    if (!current.has(id) && isLive(agent.status)) seen.set(id, { ...agent, status: 'completed' })
  })
  return [...seen.values()]
}

const elapsedOf = (id: string, now: number) => {
  const record = lifecycle.get(id)
  return record && (record.endedAt || now) - record.startedAt
}

const revealOf = (id: string) => (isDismissed(id) ? 0 : 1)

export const buildTree = (agents: readonly AgentInfo[], usage: Record<string, AgentUsage>, activity: Record<string, string>, goals: Record<string, string>, progress: Record<string, GoalProgress>, todos: Record<string, Todo[]>, models: Record<string, string>, mainFinished: boolean, now: number): Node => {
  const nodes = new Map<string, Node>(
    agents.map(a => [a.id, { id: a.id, label: `${agentName(a.id)} · ${a.type}`, status: a.status, usage: usage[a.id], model: usage[a.id]?.model ?? models[a.id], activity: activity[a.id], goal: goals[a.id], progress: progress[a.id], todos: todos[a.id], elapsed: elapsedOf(a.id, now), reveal: revealOf(a.id), children: [] }]),
  )
  const root: Node = { id: MAIN, label: 'main', status: 'running', finished: mainFinished, usage: usage[MAIN], model: usage[MAIN]?.model, activity: activity[MAIN], goal: goals[MAIN], progress: progress[MAIN], todos: todos[MAIN], elapsed: elapsedOf(MAIN, now), reveal: 1, children: [] }
  agents.forEach(a => {
    const parent = (a.parentId && nodes.get(a.parentId)) || root
    nodes.get(a.id)!.launchedBy = parent.label
    parent.children.push(nodes.get(a.id)!)
  })
  return { ...root, children: root.children.map(pruneInactive).filter((c): c is Node => !!c) }
}
const pruneInactive = (node: Node): Node | undefined => {
  const children = node.children.map(pruneInactive).filter((c): c is Node => !!c)
  return node.reveal > 0 || children.length ? { ...node, children } : undefined
}
export type Row = { node: Node; depth: number; last: boolean }
export const flatten = (node: Node, depth = 0, last = true): Row[] => [
  { node, depth, last },
  ...[...node.children].reverse().flatMap((child, index, siblings) => flatten(child, depth + 1, index === siblings.length - 1)),
]

export const sumTree = (node: Node): AgentUsage | undefined =>
  [node.usage, ...node.children.map(sumTree)].reduce<AgentUsage | undefined>(
    (acc, u) => (u ? { ...(acc ?? u), input: (acc?.input ?? 0) + u.input, output: (acc?.output ?? 0) + u.output, cacheRead: (acc?.cacheRead ?? 0) + u.cacheRead, cacheWrite: (acc?.cacheWrite ?? 0) + u.cacheWrite, cost: (acc?.cost ?? 0) + u.cost, steps: (acc?.steps ?? 0) + u.steps } : acc),
    undefined,
  )
