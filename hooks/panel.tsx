import { DANGER_FROM, RATE_LABEL_WIDTH, barLine, formatCost, formatDuration, formatTokens, levelColor, shortModel } from './format'
import { frameTime } from './clock'
import { PALETTE } from './palette'
import { makeBadge } from './badge'
import { ICONS } from './icons'
import { gradient, speciesColor, spriteLines } from './sprites'
import { MAIN, isLive } from './tree'
import type { Node } from './tree'
import type { Todo } from './activity'

export type Ui = Record<'Box' | 'Text' | 'Button', (props: any) => any>

const PCT_WIDTH = 4
const QUOTA_GAP = 2
const QUOTA_GAP_TEXT = ' '.repeat(QUOTA_GAP)
const MIN_BAR_CELLS = 10
const CARD_PADDING_X = 2
const CARD_PADDING_Y = 1
const INDENT = 2
const CONNECTOR_MIDDLE = '├─ '
const CONNECTOR_LAST = '└─ '
const SPRITE_GAP = 2
const GOAL_MARGIN_Y = 1
const SECTION_MARGIN_TOP = 2
const SPINNER_FRAMES = [0xee06, 0xee07, 0xee08, 0xee09, 0xee0a, 0xee0b].map(code => String.fromCodePoint(code))
const SPINNER_MS = 120
const spinnerFrame = (now: number) => SPINNER_FRAMES[Math.floor(now / SPINNER_MS) % SPINNER_FRAMES.length]
const TODO_MAX_LINES = 8
const TODO_MARKS = { completed: { mark: ICONS.done, color: PALETTE.muted }, pending: { mark: ICONS.pending, color: PALETTE.muted } }
const NO_GOAL_PLACEHOLDER = 'no goal yet…'
export const RULE = '─'
export const MAX_RULE_CELLS = 400

const STATUSES = {
  done: { label: 'terminé', mark: ICONS.done, background: PALETTE.badgeGreen, color: PALETTE.onBadge },
  running: { label: 'en cours', mark: ICONS.running, background: PALETTE.yellow, color: PALETTE.onYellow },
  failed: { label: 'échec', mark: ICONS.failed, background: PALETTE.badgeRed, color: PALETTE.onBadge },
}
const statusOf = ({ status, finished }: Node) => (status === 'failed' || status === 'killed' ? STATUSES.failed : isLive(status) && !finished ? STATUSES.running : STATUSES.done)

const ACTION_GAP = 4
const TINT_STEPS = 11
const doneTint = (model: string) => gradient(PALETTE.card, speciesColor(model), TINT_STEPS)[1]
export const makePanel = ({ Box, Text, Button }: Ui) => {
  const { ActionButton } = makeBadge({ Box, Text, Button })

  const Section = ({ title, icon, aside, children }: { title: string; icon: string; aside?: unknown; children: unknown }) => (
    <Box flexDirection="column" marginTop={SECTION_MARGIN_TOP}>
      <Box columnGap={1}>
        <Box flexShrink={0}><Text bold color={PALETTE.text} backgroundColor={PALETTE.surface}>{` ${icon} ${title} `}</Text></Box>
        <Box width={1} flexGrow={1} height={1} overflow="hidden"><Text color={PALETTE.rule}>{RULE.repeat(MAX_RULE_CELLS)}</Text></Box>
        {aside}
      </Box>
      <Box flexDirection="column" marginTop={1}>{children}</Box>
    </Box>
  )

  const Quota = ({ label, percent, reset }: { label: string; percent: number; reset: string }) => {
    const danger = percent >= DANGER_FROM
    return (
      <Box columnGap={QUOTA_GAP} flexWrap="wrap">
        <Box flexShrink={0}><Text color={PALETTE.muted}>{label.padEnd(RATE_LABEL_WIDTH)}</Text></Box>
        <Box width={MIN_BAR_CELLS} flexGrow={1} height={1} overflow="hidden">
          <Box width={`${percent}%`} flexShrink={0} height={1} overflow="hidden"><Text color={levelColor(percent)}>{RULE.repeat(MAX_RULE_CELLS)}</Text></Box>
          <Box width={1} flexGrow={1} height={1} overflow="hidden"><Text color={PALETTE.rule}>{RULE.repeat(MAX_RULE_CELLS)}</Text></Box>
        </Box>
        <Box flexShrink={0}>
          <Text bold={danger} color={danger ? PALETTE.red : PALETTE.text}>{`${percent}%`.padStart(PCT_WIDTH)}</Text>
          {reset && <Text color={PALETTE.muted}>{QUOTA_GAP_TEXT}{reset}</Text>}
        </Box>
      </Box>
    )
  }

  const Sprite = ({ node }: { node: Node }) => (
    <Box flexDirection="column" marginRight={SPRITE_GAP} flexShrink={0}>
      {spriteLines(node.model ?? '', node.finished ? undefined : node.activity, frameTime(), node.finished).map(line => (
        <Text>{line.map(({ char, fg, bg }) => <Text color={fg} backgroundColor={bg}>{char}</Text>)}</Text>
      ))}
    </Box>
  )

  const TodoList = ({ todos, settled }: { todos: Todo[]; settled: boolean }) => (
    <Box flexDirection="column" marginBottom={GOAL_MARGIN_Y}>
      {todos.slice(0, TODO_MAX_LINES).map(({ content, status }) => {
        const { mark, color } = settled ? TODO_MARKS.completed : status === 'in_progress' ? { mark: spinnerFrame(frameTime()), color: PALETTE.text } : (TODO_MARKS[status as keyof typeof TODO_MARKS] ?? TODO_MARKS.pending)
        return <Text wrap="truncate-end" color={color}>{mark} <Text color={settled || status === 'completed' ? PALETTE.muted : PALETTE.text}>{content}</Text></Text>
      })}
    </Box>
  )

  const AgentCard = ({ node, depth, last, route, onOpen, onDismiss }: { node: Node; depth: number; last: boolean; route?: string; onOpen: (id: string) => void; onDismiss: (id: string) => void }) => {
    const status = statusOf(node)
    const done = status === STATUSES.done
    return (
      <Box flexDirection="column" backgroundColor={done ? doneTint(node.usage?.model ?? node.model ?? '') : PALETTE.card} marginLeft={depth * INDENT} marginBottom={1} paddingX={CARD_PADDING_X} paddingY={CARD_PADDING_Y} overflow="hidden">
        <Box alignItems="center">
          <Sprite node={node} />
          <Box flexDirection="column" flexGrow={1}>
            <Box justifyContent="space-between" flexWrap="wrap" columnGap={2}>
              <Text>
                {depth > 0 && <Text color={PALETTE.muted}>{last ? CONNECTOR_LAST : CONNECTOR_MIDDLE}</Text>}
                <Text bold color={PALETTE.text}>{node.label}</Text>{'  '}
                <Text bold backgroundColor={status.background} color={status.color}>{` ${status.label} `}</Text>
                {node.elapsed !== undefined && <Text color={PALETTE.muted}>  {done ? 'en ' : ''}{formatDuration(node.elapsed)}</Text>}
              </Text>
              {node.usage && <Text color={PALETTE.muted}>{shortModel(node.usage.model)}{route && ` · ${route}`}  {formatCost(node.usage.cost)}</Text>}
            </Box>
            {node.id !== MAIN && node.launchedBy && <Text color={PALETTE.muted}>lancé par {node.launchedBy}</Text>}
            <Box marginY={GOAL_MARGIN_Y}>
              <Text bold color={PALETTE.text}>{done ? ICONS.done : ICONS.goal} {node.goal ?? NO_GOAL_PLACEHOLDER}</Text>
            </Box>
            {!!node.todos?.length && <TodoList todos={node.todos} settled={done} />}
            <Box justifyContent="space-between" alignItems="center">
              <Box>
                {node.usage && (
                  <Text>
                    <Text color={PALETTE.muted}>{ICONS.download} {formatTokens(node.usage.input + node.usage.cacheRead + node.usage.cacheWrite)}  {ICONS.upload} {formatTokens(node.usage.output)}  {ICONS.requests} {node.usage.steps}</Text>
                    {isLive(node.status) && !!node.progress?.total && <Text color={PALETTE.muted}>  étape {barLine(node.progress).label}</Text>}
                  </Text>
                )}
              </Box>
              {node.id !== MAIN && (
                <Box columnGap={ACTION_GAP}>
                  <ActionButton id={`open:${node.id}`} label="ouvrir ▸" background={PALETTE.rule} onPress={() => onOpen(node.id)} />
                  {!isLive(node.status) && <ActionButton id={`dismiss:${node.id}`} label={`retirer ${ICONS.failed}`} background={PALETTE.badgeRed} onPress={() => onDismiss(node.id)} />}
                </Box>
              )}
            </Box>
          </Box>
        </Box>
      </Box>
    )
  }

  return { Section, Quota, AgentCard, Sprite }
}
