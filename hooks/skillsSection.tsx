import { PALETTE } from './palette'
import { makeBadge } from './badge'
import { skillsSummary, viewSkills } from './skillsView'
import type { SkillRow } from './skillsView'
import type { SkillLedger, SkillStatus } from './skillTypes'

type Ui = Record<'Box' | 'Text' | 'Button', (props: any) => any>
type Handlers = { onAccept: (slug: string) => void; onReject: (slug: string) => void; onRestore: (slug: string) => void; onRevalidate: (slug: string) => void; onDismiss: (slug: string) => void; onDismissInactive: () => void; isBusy: (slug: string) => boolean }

const MAX_VISIBLE_ROWS = 6
const ACTION_GAP = 2
const ROW_GAP = 2
const RAIL_WIDTH = 1
const DISMISS_LABEL = 'dégager ✕'
const INACTIVE_STATUSES: SkillStatus[] = ['stale', 'archived', 'rejected']
const BUSY_LABEL = 'rédaction…'
const RETRY_LABEL = 'réessayer'
const FAILED_PREFIX = 'échec : '
const EMPTY_LABEL = 'aucun skill appris pour le moment'

const STATUS_BADGES: Record<SkillStatus, { label: string; background: string; color: string }> = {
  active: { label: 'actif', background: PALETTE.badgeGreen, color: PALETTE.onBadge },
  stale: { label: 'périmé', background: PALETTE.yellow, color: PALETTE.onYellow },
  candidate: { label: 'candidat', background: PALETTE.rule, color: PALETTE.text },
  archived: { label: 'archivé', background: PALETTE.surface, color: PALETTE.muted },
  rejected: { label: 'rejeté', background: PALETTE.surface, color: PALETTE.muted },
}

const CARD_TINTS: Record<SkillStatus, { rail: string; background: string }> = {
  candidate: { rail: PALETTE.yellow, background: '#2a2819' },
  active: { rail: PALETTE.green, background: '#1d2a20' },
  stale: { rail: '#8a7a3a', background: '#262520' },
  archived: { rail: PALETTE.rule, background: PALETTE.card },
  rejected: { rail: PALETTE.rule, background: PALETTE.card },
}

const isInactive = (row: SkillRow) => INACTIVE_STATUSES.includes(row.status)

export const makeSkillsSection = ({ Box, Text, Button }: Ui) => {
  const { ActionButton, Badge } = makeBadge({ Box, Text, Button })

  const RowActions = ({ row, onAccept, onReject, onRestore, onRevalidate, onDismiss, isBusy }: { row: SkillRow } & Handlers) => {
    const { slug, status } = row
    if (isInactive(row)) {
      const isStale = status === 'stale'
      return (
        <>
          <ActionButton id={`${isStale ? 'revalidate' : 'restore'}:${slug}`} label={isStale ? 'revalider' : 'restaurer'} background={PALETTE.rule} onPress={() => (isStale ? onRevalidate(slug) : onRestore(slug))} />
          <ActionButton id={`dismiss:${slug}`} label={DISMISS_LABEL} background={PALETTE.badgeRed} onPress={() => onDismiss(slug)} />
        </>
      )
    }
    const isCandidate = status === 'candidate'
    if (isBusy(slug)) return <Text color={PALETTE.muted}>{BUSY_LABEL}</Text>
    return (
      <>
        {isCandidate && <ActionButton id={`accept:${slug}`} label={row.lastError ? RETRY_LABEL : 'accepter'} background={PALETTE.badgeGreen} onPress={() => onAccept(slug)} />}
        <ActionButton id={`reject:${slug}`} label={isCandidate ? 'rejeter' : 'supprimer'} background={PALETTE.badgeRed} onPress={() => onReject(slug)} />
      </>
    )
  }

  const SkillRowView = ({ row, ...handlers }: { row: SkillRow } & Handlers) => {
    const badge = STATUS_BADGES[row.status]
    const tint = CARD_TINTS[row.status]
    return (
      <Box marginTop={1}>
        <Box width={RAIL_WIDTH} flexShrink={0} backgroundColor={tint.rail} />
        <Box flexDirection="column" flexGrow={1} paddingX={1} backgroundColor={tint.background}>
        <Box justifyContent="space-between" columnGap={ROW_GAP}>
          <Box columnGap={ROW_GAP} flexShrink={1}>
            <Badge background={badge.background} color={badge.color}>{badge.label}</Badge>
            <Text bold wrap="truncate-end" color={PALETTE.text}>{row.slug}</Text>
            <Text color={PALETTE.muted}>{`${row.cleanUses}/${row.uses}`}</Text>
          </Box>
          <Box columnGap={ACTION_GAP} flexShrink={0}><RowActions row={row} {...handlers} /></Box>
        </Box>
        <Text color={PALETTE.muted} wrap="truncate-end">{row.summary}</Text>
        {row.lastError && <Text color={PALETTE.red} wrap="truncate-end">{`${FAILED_PREFIX}${row.lastError}`}</Text>}
        </Box>
      </Box>
    )
  }

  return ({ ledger, ...handlers }: { ledger: SkillLedger } & Handlers) => {
    const rows = viewSkills(ledger)
    const hidden = rows.length - MAX_VISIBLE_ROWS
    const inactiveCount = rows.filter(isInactive).length
    if (rows.length === 0) return <Text color={PALETTE.muted}>{EMPTY_LABEL}</Text>
    return (
      <Box flexDirection="column">
        <Text color={PALETTE.muted}>{skillsSummary(ledger)}</Text>
        {rows.slice(0, MAX_VISIBLE_ROWS).map(row => <SkillRowView row={row} {...handlers} />)}
        {hidden > 0 && <Text color={PALETTE.muted}>+{hidden} more</Text>}
        {inactiveCount > 0 && (
          <Box justifyContent="flex-end" marginTop={1}>
            <ActionButton id="dismiss-all" label={`tout dégager (${inactiveCount}) ✕`} background={PALETTE.badgeRed} onPress={handlers.onDismissInactive} />
          </Box>
        )}
      </Box>
    )
  }
}
