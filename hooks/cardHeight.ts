import { SPRITE_CELLS_Y } from './sprites'
import { MAIN } from './tree'
import type { Node } from './tree'

export const CARD_PADDING_Y = 1
export const GOAL_MARGIN_Y = 1
export const CARD_MARGIN_BOTTOM = 1
export const TODO_MAX_LINES = 8
const HEADER_LINES = 1
const GOAL_LINES = 1
const COMPACT_CONTENT_LINES = 3
const FOOTER_LINES = 1

const todoLines = ({ todos }: Node) => (todos?.length ? Math.min(todos.length, TODO_MAX_LINES) + GOAL_MARGIN_Y : 0)

export const cardHeight = (node: Node) => {
  if (node.id !== MAIN) return COMPACT_CONTENT_LINES + CARD_MARGIN_BOTTOM
  const content = HEADER_LINES + 2 * GOAL_MARGIN_Y + GOAL_LINES + todoLines(node) + FOOTER_LINES
  return 2 * CARD_PADDING_Y + Math.max(SPRITE_CELLS_Y, content) + CARD_MARGIN_BOTTOM
}
