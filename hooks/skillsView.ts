import type { SkillLedger, SkillRecord, SkillStatus } from './skillTypes'

export type SkillRow = {
  slug: string
  status: SkillStatus
  uses: number
  cleanUses: number
  idleSessions: number
  label: string
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

const lastUsed = (record: SkillRecord) => record.lastUsedAt ?? 0

const compareRecords = (a: SkillRecord, b: SkillRecord) =>
  STATUS_ORDER.indexOf(a.status) - STATUS_ORDER.indexOf(b.status) || lastUsed(b) - lastUsed(a) || a.slug.localeCompare(b.slug)

const toRow = ({ slug, status, uses, cleanUses, idleSessions }: SkillRecord): SkillRow => ({
  slug,
  status,
  uses,
  cleanUses,
  idleSessions,
  label: `${slug} · ${cleanUses}/${uses}`,
})

export const viewSkills = (ledger: SkillLedger): SkillRow[] => Object.values(ledger).sort(compareRecords).map(toRow)

export const skillsSummary = (ledger: SkillLedger): string => {
  const records = Object.values(ledger)
  return SUMMARY_PARTS.map(([status, name]) => [records.filter(record => record.status === status).length, name] as const)
    .filter(([count]) => count > 0)
    .map(([count, name]) => `${count} ${name}`)
    .join(SUMMARY_SEPARATOR)
}
