import { isLive } from './tree'
import type { Node } from './tree'

export type CardKind = 'running' | 'paused' | 'done' | 'failed' | 'gone'

const IDLE = 'idle'
const COMPLETED = 'completed'

const hasLiveDescendant = ({ children }: Node): boolean => children.some(child => isLive(child.status) || hasLiveDescendant(child))

const workIsOver = ({ status, todos, progress }: Node) => {
  if (todos?.length) return todos.every(todo => todo.status === COMPLETED)
  if (status === IDLE) return false
  return !progress?.total || progress.done >= progress.total
}

export const cardKind = (node: Node): CardKind => {
  if (node.status === 'gone') return 'gone'
  if (node.status === 'failed' || node.status === 'killed') return 'failed'
  if (isLive(node.status) && !node.finished) return 'running'
  return workIsOver(node) && !hasLiveDescendant(node) ? 'done' : 'paused'
}
