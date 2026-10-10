import { PALETTE } from './palette'
import { gradient } from './sprites'
import type { Ui } from './panel'

const LOGO = [
  '██╗  ██╗██╗██╗   ██╗███████╗',
  '██║  ██║██║██║   ██║██╔════╝',
  '███████║██║██║   ██║█████╗  ',
  '██╔══██║██║╚██╗ ██╔╝██╔══╝  ',
  '██║  ██║██║ ╚████╔╝ ███████╗',
  '╚═╝  ╚═╝╚═╝  ╚═══╝  ╚══════╝',
]
const COMPACT_TITLE = '⬡ HIVE'
const TAGLINE = '── agents en essaim ──'

export const makeBanner = ({ Box, Text }: Ui) => {
  const Banner = ({ compact }: { compact?: boolean }) => {
    if (compact) return <Box justifyContent="center"><Text bold color={PALETTE.yellow}>{COMPACT_TITLE}</Text><Text color={PALETTE.muted}> {TAGLINE}</Text></Box>
    const colors = gradient(PALETTE.yellow, PALETTE.green, LOGO.length)
    return (
      <Box flexDirection="column" alignItems="center">
        {LOGO.map((line, row) => <Text bold color={colors[row]}>{line}</Text>)}
        <Text color={PALETTE.muted}>{TAGLINE}</Text>
      </Box>
    )
  }
  return { Banner }
}
