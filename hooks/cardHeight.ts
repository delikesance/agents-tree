import { SPRITE_CELLS_Y } from './sprites'
import type { Node } from './tree'

export const CARD_PADDING_Y = 1
export const GOAL_MARGIN_Y = 1
export const CARD_MARGIN_BOTTOM = 1
export const TODO_MAX_LINES = 8
const HEADER_LINES = 1
const LAUNCHER_LINES = 1
const GOAL_LINES = 1
const FOOTER_LINES = 1

const launcherLines = ({ launchedBy }: Node) => (launchedBy ? LAUNCHER_LINES : 0)
const todoLines = ({ todos }: Node) => (todos?.length ? Math.min(todos.length, TODO_MAX_LINES) + GOAL_MARGIN_Y : 0)

export const cardHeight = (node: Node, compact: boolean) => {
  if (compact) return HEADER_LINES + launcherLines(node) + GOAL_LINES + FOOTER_LINES
  const content = HEADER_LINES + launcherLines(node) + 2 * GOAL_MARGIN_Y + GOAL_LINES + todoLines(node) + FOOTER_LINES
  return 2 * CARD_PADDING_Y + Math.max(SPRITE_CELLS_Y, content) + CARD_MARGIN_BOTTOM
}
