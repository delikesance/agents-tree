import { isLive } from './tree'
import type { Node } from './tree'

export type CardKind = 'running' | 'paused' | 'done' | 'failed' | 'gone'

const IDLE = 'idle'
const COMPLETED = 'completed'

const hasLiveDescendant = ({ children }: Node): boolean => children.some(child => isLive(child.status) || hasLiveDescendant(child))

const isFinished = ({ status, finished }: Node) => Boolean(finished) || status === COMPLETED || status === IDLE

const workIsOver = (node: Node) => {
  if (isFinished(node)) return true
  const { todos, progress } = node
  if (todos?.length) return todos.every(todo => todo.status === COMPLETED)
  return !progress?.total || progress.done >= progress.total
}

export const cardKind = (node: Node): CardKind => {
  if (node.status === 'gone') return 'gone'
  if (node.status === 'failed' || node.status === 'killed') return 'failed'
  if (isLive(node.status) && !node.finished) return 'running'
  return workIsOver(node) && !hasLiveDescendant(node) ? 'done' : 'paused'
}
