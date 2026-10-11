import type { SkillLedger, SkillRecord, SkillStatus } from './skillTypes'

export type SkillRow = {
  slug: string
  status: SkillStatus
  uses: number
  cleanUses: number
  idleSessions: number
  label: string
  summary: string
}

const STATUS_ORDER: SkillStatus[] = ['stale', 'candidate', 'active', 'archived', 'rejected']
const SUMMARY_PARTS: [SkillStatus, string][] = [
  ['active', 'active'],
  ['stale', 'stale'],
  ['candidate', 'candidates'],
  ['archived', 'archived'],
  ['rejected', 'rejected'],
]
const SUMMARY_SEPARATOR = ' · '
const SUMMARY_MAX_CHARS = 80
const ELLIPSIS = '…'

const lastUsed = (record: SkillRecord) => record.lastUsedAt ?? 0

const compareRecords = (a: SkillRecord, b: SkillRecord) =>
  STATUS_ORDER.indexOf(a.status) - STATUS_ORDER.indexOf(b.status) || lastUsed(b) - lastUsed(a) || a.slug.localeCompare(b.slug)

const collapseWhitespace = (text: string) => text.replace(/\s+/g, ' ').trim()

const truncate = (text: string) =>
  text.length > SUMMARY_MAX_CHARS ? `${text.slice(0, SUMMARY_MAX_CHARS - ELLIPSIS.length)}${ELLIPSIS}` : text

const describe = ({ signals, tags }: SkillRecord) => {
  const note = signals.map(signal => collapseWhitespace(signal.note)).find(Boolean)
  const kinds = [...new Set(signals.map(signal => signal.kind))].join(', ')
  return note || kinds || tags.join(', ')
}

const summarize = (record: SkillRecord) => truncate(collapseWhitespace(describe(record)))

const toRow = (record: SkillRecord): SkillRow => ({
  slug: record.slug,
  status: record.status,
  uses: record.uses,
  cleanUses: record.cleanUses,
  idleSessions: record.idleSessions,
  label: `${record.slug} · ${record.cleanUses}/${record.uses}`,
  summary: summarize(record),
})

export const viewSkills = (ledger: SkillLedger): SkillRow[] => Object.values(ledger).sort(compareRecords).map(toRow)

export const skillsSummary = (ledger: SkillLedger): string => {
  const records = Object.values(ledger)
  return SUMMARY_PARTS.map(([status, name]) => [records.filter(record => record.status === status).length, name] as const)
    .filter(([count]) => count > 0)
    .map(([count, name]) => `${count} ${name}`)
    .join(SUMMARY_SEPARATOR)
}
