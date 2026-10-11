import { agentName } from './names'
import { cardHeight } from './cardHeight'
import { forgetDismissed, isDismissed } from './select'
import type { AgentInfo } from 'claude-code'
import type { AgentUsage, GoalProgress } from '../types'
import type { Todo } from './activity'
import { groupByRun, workflowLabel, workflowNodeId } from './workflowRuns'
import type { WorkflowAgent } from './workflowRuns'

export const MAIN = 'main'
export const isLive = (status: string) => status === 'running' || status === 'waiting' || status === 'pending'

export type Node = { id: string; label: string; status: string; usage?: AgentUsage; activity?: string; model?: string; goal?: string; progress?: GoalProgress; todos?: Todo[]; elapsed?: number; reveal: number; finished?: boolean; launchedBy?: string; children: Node[] }

export type TrackedAgent = Omit<AgentInfo, 'status'> & { status: string }

type Lifecycle = { startedAt: number; endedAt?: number }
const lifecycle = new Map<string, Lifecycle>()

export const trackLifecycle = (agents: readonly TrackedAgent[], now: number) => {
  if (!lifecycle.has(MAIN)) lifecycle.set(MAIN, { startedAt: now })
  agents.forEach(({ id, status }) => {
    const record = lifecycle.get(id) ?? { startedAt: now, endedAt: isLive(status) ? undefined : 0 }
    if (!isLive(status) && record.endedAt === undefined) record.endedAt = now
    lifecycle.set(id, record)
  })
}

const GONE = 'gone'
const seen = new Map<string, TrackedAgent>()

export const rememberAgents = (agents: readonly TrackedAgent[]): TrackedAgent[] => {
  const current = new Set(agents.map(({ id }) => id))
  seen.forEach((agent, id) => {
    if (current.has(id) || agent.status !== GONE) return
    ;[seen, lifecycle].forEach(store => store.delete(id))
    forgetDismissed(id)
  })
  agents.forEach(agent => seen.set(agent.id, agent))
  seen.forEach((agent, id) => {
    if (!current.has(id) && isLive(agent.status)) seen.set(id, { ...agent, status: GONE })
  })
  return [...seen.values()]
}

const elapsedOf = (id: string, now: number) => {
  const record = lifecycle.get(id)
  return record && (record.endedAt || now) - record.startedAt
}

const revealOf = (id: string) => (isDismissed(id) ? 0 : 1)

const workflowStatus = ({ endedAt }: WorkflowAgent) => (endedAt === undefined ? 'running' : 'completed')

const workflowGroup = (runId: string, members: Node[]): Node => {
  const id = workflowNodeId(runId)
  const group: Node = { id, label: workflowLabel(runId), status: members.some(m => isLive(m.status)) ? 'running' : 'completed', reveal: revealOf(id), children: members }
  return { ...group, usage: sumTree(group) }
}

export const buildTree = (agents: readonly TrackedAgent[], usage: Record<string, AgentUsage>, activity: Record<string, string>, goals: Record<string, string>, progress: Record<string, GoalProgress>, todos: Record<string, Todo[]>, models: Record<string, string>, mainFinished: boolean, now: number, workflow: readonly WorkflowAgent[] = [], launches: readonly number[] = []): Node => {
  const nodeFor = (id: string, label: string, status: string, elapsed: number | undefined): Node => ({ id, label, status, usage: usage[id], model: usage[id]?.model ?? models[id], activity: activity[id], goal: goals[id], progress: progress[id], todos: todos[id], elapsed, reveal: revealOf(id), children: [] })
  const nodes = new Map<string, Node>(agents.map(a => [a.id, nodeFor(a.id, `${agentName(a.id)} · ${a.type}`, a.status, elapsedOf(a.id, now))]))
  const root: Node = { id: MAIN, label: 'main', status: 'running', finished: mainFinished, usage: usage[MAIN], model: usage[MAIN]?.model, activity: activity[MAIN], goal: goals[MAIN], progress: progress[MAIN], todos: todos[MAIN], elapsed: elapsedOf(MAIN, now), reveal: 1, children: [] }
  agents.forEach(a => {
    const parent = (a.parentId && nodes.get(a.parentId)) || root
    if (parent !== root) nodes.get(a.id)!.launchedBy = parent.label
    parent.children.push(nodes.get(a.id)!)
  })
  groupByRun(workflow).forEach((members, runId) => {
    const children = members.map(m => ({ ...nodeFor(m.id, `${agentName(m.id)} · ${m.type}`, workflowStatus(m), (m.endedAt ?? now) - m.startedAt), launchedBy: workflowLabel(runId) }))
    root.children.push(workflowGroup(runId, children))
  })
  launches.forEach(at => {
    const id = workflowNodeId(`starting_${at}`)
    root.children.push({ id, label: 'workflow · démarrage', status: 'running', elapsed: now - at, reveal: revealOf(id), children: [] })
  })
  return { ...root, children: root.children.map(pruneInactive).filter((c): c is Node => !!c) }
}
const pruneInactive = (node: Node): Node | undefined => {
  const children = node.children.map(pruneInactive).filter((c): c is Node => !!c)
  return node.reveal > 0 || children.length ? { ...node, children } : undefined
}
export type Row = { node: Node; depth: number; last: boolean; ancestorsLast: boolean[] }
export const flatten = (node: Node, depth = 0, last = true, ancestorsLast: boolean[] = []): Row[] => [
  { node, depth, last, ancestorsLast },
  ...[...node.children].reverse().flatMap((child, index, siblings) => flatten(child, depth + 1, index === siblings.length - 1, depth > 0 ? [...ancestorsLast, last] : ancestorsLast)),
]

const isActive = ({ node }: Row) => node.id === MAIN || isLive(node.status)
const parentIndexes = (rows: readonly Row[]) => {
  const path: number[] = []
  return rows.map(({ depth }, index) => {
    path.length = depth
    const parent = path[depth - 1] ?? -1
    path[depth] = index
    return parent
  })
}
const totalHeight = (rows: readonly Row[], compact: boolean) => rows.reduce((sum, { node }) => sum + cardHeight(node, compact), 0)
export const needsCompact = (rows: readonly Row[], maxRows: number) => totalHeight(rows, false) > maxRows

export function visibleAgents<T extends Row>(nodes: readonly T[], maxRows: number): { shown: T[]; hidden: number } {
  const compact = needsCompact(nodes, maxRows)
  const parents = parentIndexes(nodes)
  const kept = new Set<number>()
  let used = 0
  const heightAt = (index: number) => {
    const row = nodes[index]
    return row ? cardHeight(row.node, compact) : 0
  }
  const keep = (index: number) => {
    for (let i = index; i >= 0 && !kept.has(i); i = parents[i] ?? -1) {
      kept.add(i)
      used += heightAt(i)
    }
  }
  const costOf = (index: number) => {
    let cost = 0
    for (let i = index; i >= 0 && !kept.has(i); i = parents[i] ?? -1) cost += heightAt(i)
    return cost
  }
  nodes.forEach((row, index) => isActive(row) && keep(index))
  nodes.forEach((_, index) => {
    if (!kept.has(index) && used + costOf(index) <= maxRows) keep(index)
  })
  return { shown: nodes.filter((_, index) => kept.has(index)), hidden: nodes.length - kept.size }
}

export const sumTree = (node: Node): AgentUsage | undefined =>
  [node.usage, ...node.children.map(sumTree)].reduce<AgentUsage | undefined>(
    (acc, u) => (u ? { ...(acc ?? u), input: (acc?.input ?? 0) + u.input, output: (acc?.output ?? 0) + u.output, cacheRead: (acc?.cacheRead ?? 0) + u.cacheRead, cacheWrite: (acc?.cacheWrite ?? 0) + u.cacheWrite, cost: (acc?.cost ?? 0) + u.cost, steps: (acc?.steps ?? 0) + u.steps } : acc),
    undefined,
  )
