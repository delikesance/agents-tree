import { clipLines } from './format'
import { makeBadge } from './badge'
import { ICONS, errorIcon, toolIcon } from './icons'
import { PALETTE } from './palette'
import type { ThreadLine } from './threadLines'

type Ui = Record<'Box' | 'Text' | 'Button' | 'Input', (props: any) => any>

const LINE_MAX_LINES = 3
const MARK_WIDTH = 2
const LINE_CHROME = 6
const PILL_CHROME = 5
const ERROR_LABEL_WIDTH = 6
const PROMPT_LABEL_WIDTH = 2
const PROMPT_MAX_LINES = 6
const PANE_PADDING = 2

export const makeThread = ({ Box, Text, Button, Input }: Ui) => {
  const { ActionButton } = makeBadge({ Box, Text, Button })

  const Pill = ({ icon, name, background }: { icon: string; name: string; background: string }) => (
    <Text bold backgroundColor={background} color={PALETTE.onBadge}>{` ${icon} ${name} `}</Text>
  )

  const Line = ({ kind, text, tool = '', cols }: ThreadLine & { cols: number }) => {
    const width = cols - PANE_PADDING - MARK_WIDTH - LINE_CHROME
    const argument = clipLines(text.slice(tool.length).trim(), width - tool.length - PILL_CHROME - (kind === 'error' ? ERROR_LABEL_WIDTH : 0), LINE_MAX_LINES)
    if (kind === 'tool') return <Text color={PALETTE.muted}><Pill icon={toolIcon(tool)} name={tool.toLowerCase()} background={PALETTE.rule} /> {argument}</Text>
    if (kind === 'error') return <Box backgroundColor={PALETTE.errorRow} paddingX={1}><Text color={PALETTE.onBadge}><Pill icon={errorIcon()} name={tool.toLowerCase()} background={PALETTE.badgeRed} /> <Text color={PALETTE.red}>échec</Text> {argument}</Text></Box>
    const user = kind === 'user'
    return (
      <Box backgroundColor={user ? PALETTE.surface : undefined} paddingX={user ? 1 : 0} marginBottom={1}>
        <Text color={PALETTE.text}>{user ? ICONS.prompt_user : ICONS.assistant} {clipLines(text, width, LINE_MAX_LINES)}</Text>
      </Box>
    )
  }

  const Thread = ({ title, stats, prompt, avatar, lines, cols, onBack, onSend }: { title: string; stats: string[]; prompt?: string; avatar: unknown; lines: ThreadLine[]; cols: number; onBack: () => void; onSend: (text: string) => void }) => (
    <Box flexDirection="column" padding={1}>
      <Box columnGap={2} justifyContent="space-between">
        <Box flexDirection="column">
          <Box columnGap={2}>
            <ActionButton id="back" label={`${ICONS.back} retour`} background={PALETTE.badgeRed} onPress={onBack} />
            <Text bold color={PALETTE.text}>{title}</Text>
          </Box>
          {stats.map(line => <Box alignSelf="flex-start" backgroundColor={PALETTE.surface} paddingX={1}><Text color={PALETTE.muted}>{line}</Text></Box>)}
        </Box>
        {avatar}
      </Box>
      {prompt && (
        <Box marginTop={1} backgroundColor={PALETTE.surface} paddingX={1}>
          <Text color={PALETTE.muted}>{ICONS.prompt} <Text color={PALETTE.text}>{clipLines(prompt, cols - PANE_PADDING - PROMPT_LABEL_WIDTH - 2, PROMPT_MAX_LINES)}</Text></Text>
        </Box>
      )}
      <Box flexDirection="column" marginY={1} borderStyle="single" borderColor={PALETTE.rule} paddingX={1}>
        {lines.length ? lines.map(line => <Line {...line} cols={cols} />) : <Text color={PALETTE.muted}>aucun message</Text>}
      </Box>
      <Input key="reply" placeholder={`écrire à ${title}…`} submitLabel="envoyer" autoFocus onSubmit={onSend} />
    </Box>
  )

  return { Thread }
}
