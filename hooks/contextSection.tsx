import { clampedPercent, legendSegments, levelColor } from './format'
import { PALETTE } from './palette'
import { MAX_RULE_CELLS, RULE } from './panel'
import type { Ui } from './panel'

type Segment = { name: string; tokens: string; percent: number; color: string }
const LEGEND_GAP = 3
const MARK = '█'
const FILL = '█'

export const makeContext = ({ Box, Text }: Ui) => {
  const Cells = ({ width, color, grow, glyph = RULE }: { width?: string; color: string; grow?: boolean; glyph?: string }) => (
    <Box width={width ?? 1} flexGrow={grow ? 1 : 0} flexShrink={0} height={1} overflow="hidden"><Text color={color}>{glyph.repeat(MAX_RULE_CELLS)}</Text></Box>
  )

  const ContextBreakdown = ({ percent, usage, segments }: { percent: number; usage: string; segments: readonly Segment[] }) => {
    const shown = segments.length ? segments : [{ name: 'contexte', tokens: usage, percent: clampedPercent(percent), color: levelColor(percent) }]
    return (
      <Box flexDirection="column">
        <Box height={1} overflow="hidden">
          {shown.map(({ percent: width, color }) => <Cells width={`${width}%`} color={color} glyph={FILL} />)}
          <Cells color={PALETTE.rule} glyph={FILL} grow />
        </Box>
        <Box columnGap={LEGEND_GAP} flexWrap="wrap" marginTop={1}>
          {legendSegments(segments).map(({ name, percent: share, color }) => <Text color={PALETTE.muted}><Text color={color}>{MARK}</Text> {name} ({share}%)</Text>)}
        </Box>
      </Box>
    )
  }

  const ContextFigure = ({ percent }: { percent: number }) => (
    <Box flexShrink={0}><Text backgroundColor={PALETTE.surface} color={PALETTE.muted}>{' '}<Text bold color={levelColor(percent)}>{percent}%</Text>{' '}</Text></Box>
  )

  return { ContextBreakdown, ContextFigure }
}
