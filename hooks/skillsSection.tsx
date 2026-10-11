import { PALETTE } from './palette'
import { makeBadge } from './badge'
import { skillsSummary, viewSkills } from './skillsView'
import type { SkillRow } from './skillsView'
import type { SkillLedger, SkillStatus } from './skillTypes'

type Ui = Record<'Box' | 'Text' | 'Button', (props: any) => any>
type Handlers = { onAccept: (slug: string) => void; onReject: (slug: string) => void; onRestore: (slug: string) => void; onRevalidate: (slug: string) => void; isBusy: (slug: string) => boolean }

const MAX_VISIBLE_ROWS = 6
const ACTION_GAP = 2
const ROW_GAP = 2
const BUSY_LABEL = 'rédaction…'
const EMPTY_LABEL = 'aucun skill appris pour le moment'

const STATUS_COLORS: Record<SkillStatus, string> = {
  active: PALETTE.badgeGreen,
  stale: PALETTE.yellow,
  candidate: PALETTE.text,
  archived: PALETTE.muted,
  rejected: PALETTE.muted,
}

export const makeSkillsSection = ({ Box, Text, Button }: Ui) => {
  const { ActionButton } = makeBadge({ Box, Text, Button })

  const RowActions = ({ row, onAccept, onReject, onRestore, onRevalidate, isBusy }: { row: SkillRow } & Handlers) => {
    const { slug, status } = row
    if (status === 'stale') return <ActionButton id={`revalidate:${slug}`} label="revalider" background={PALETTE.rule} onPress={() => onRevalidate(slug)} />
    if (status === 'archived' || status === 'rejected') return <ActionButton id={`restore:${slug}`} label="restaurer" background={PALETTE.rule} onPress={() => onRestore(slug)} />
    const isCandidate = status === 'candidate'
    if (isBusy(slug)) return <Text color={PALETTE.muted}>{BUSY_LABEL}</Text>
    return (
      <>
        {isCandidate && <ActionButton id={`accept:${slug}`} label="accepter" background={PALETTE.badgeGreen} onPress={() => onAccept(slug)} />}
        <ActionButton id={`reject:${slug}`} label={isCandidate ? 'rejeter' : 'supprimer'} background={PALETTE.badgeRed} onPress={() => onReject(slug)} />
      </>
    )
  }

  const SkillRowView = ({ row, ...handlers }: { row: SkillRow } & Handlers) => (
    <Box flexDirection="column">
      <Box justifyContent="space-between" columnGap={ROW_GAP}>
        <Text wrap="truncate-end" color={STATUS_COLORS[row.status]}>{row.label}</Text>
        <Box columnGap={ACTION_GAP} flexShrink={0}><RowActions row={row} {...handlers} /></Box>
      </Box>
      {row.summary && <Text color={PALETTE.muted} wrap="truncate-end">{row.summary}</Text>}
    </Box>
  )

  return ({ ledger, ...handlers }: { ledger: SkillLedger } & Handlers) => {
    const rows = viewSkills(ledger)
    const hidden = rows.length - MAX_VISIBLE_ROWS
    if (rows.length === 0) return <Text color={PALETTE.muted}>{EMPTY_LABEL}</Text>
    return (
      <Box flexDirection="column">
        <Text color={PALETTE.muted}>{skillsSummary(ledger)}</Text>
        {rows.slice(0, MAX_VISIBLE_ROWS).map(row => <SkillRowView row={row} {...handlers} />)}
        {hidden > 0 && <Text color={PALETTE.muted}>+{hidden} more</Text>}
      </Box>
    )
  }
}
