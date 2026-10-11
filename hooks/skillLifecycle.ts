import type { SkillLedger, SkillRecord } from './skillTypes'
import { ARCHIVE_AFTER_IDLE_SESSIONS } from './skillTypes'

const UNDERPERFORM_MIN_USES = 5
const UNDERPERFORM_MIN_CLEAN_RATIO = 0.5

const update = (ledger: SkillLedger, slug: string, change: (record: SkillRecord) => SkillRecord) =>
  ledger[slug] ? { ...ledger, [slug]: change(ledger[slug]) } : ledger

const mapRecords = (ledger: SkillLedger, change: (record: SkillRecord) => SkillRecord) =>
  Object.fromEntries(Object.entries(ledger).map(([slug, record]) => [slug, change(record)]))

const hasUseInSession = (record: SkillRecord, sessionId: string) => record.sessions.includes(sessionId)

export const recordUse = (ledger: SkillLedger, slug: string, at: number, clean: boolean, sessionId?: string) =>
  update(ledger, slug, record => ({
    ...record,
    sessions: sessionId && !hasUseInSession(record, sessionId) ? [...record.sessions, sessionId] : record.sessions,
    uses: record.uses + 1,
    cleanUses: record.cleanUses + (clean ? 1 : 0),
    idleSessions: 0,
    lastUsedAt: at,
  }))

const tick = (sessionId: string) => (record: SkillRecord): SkillRecord => {
  if (record.status !== 'active' || hasUseInSession(record, sessionId)) return record
  const idleSessions = record.idleSessions + 1
  return { ...record, idleSessions, status: idleSessions >= ARCHIVE_AFTER_IDLE_SESSIONS ? 'archived' : record.status }
}

export const endSessionTick = (ledger: SkillLedger, sessionId: string) => mapRecords(ledger, tick(sessionId))

const normalize = (path: string) => path.replace(/\/+$/, '')

const covers = (changed: string, cited: string) => {
  const [a, b] = [normalize(changed), normalize(cited)]
  return a === b || b.startsWith(`${a}/`) || a.startsWith(`${b}/`)
}

const touches = (record: SkillRecord, changedPaths: string[]) =>
  record.citedPaths.some(cited => changedPaths.some(changed => covers(changed, cited)))

export const markStale = (ledger: SkillLedger, changedPaths: string[]) =>
  mapRecords(ledger, record => (record.status === 'active' && touches(record, changedPaths) ? { ...record, status: 'stale' } : record))

export const revalidate = (ledger: SkillLedger, slug: string, headCommit: string) =>
  update(ledger, slug, record => (record.status === 'stale' ? { ...record, status: 'active', headCommit } : record))

export const underperforming = (record: SkillRecord) =>
  record.uses >= UNDERPERFORM_MIN_USES && record.cleanUses / record.uses < UNDERPERFORM_MIN_CLEAN_RATIO
