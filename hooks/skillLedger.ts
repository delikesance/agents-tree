import { tokenize } from './knowledge'
import { MAX_ACTIVE_SKILLS, PROMOTE_MIN_SESSIONS, type SkillLedger, type SkillRecord, type SkillSignal } from './skillTypes'

const MIN_JACCARD = 0.4
const MAX_TAGS = 8
const MAX_SLUG_TAGS = 3
const MAX_SLUG_CHARS = 40
const MAX_SIGNALS_PER_RECORD = 20
const FALLBACK_SLUG = 'skill'
const BLOCKED_STATUSES: SkillRecord['status'][] = ['rejected', 'archived']

export const signalTags = (signal: SkillSignal) => {
  const counts = new Map<string, number>()
  for (const token of tokenize([signal.note, ...signal.files, ...signal.tools].join(' '))) counts.set(token, (counts.get(token) ?? 0) + 1)
  return [...counts]
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .slice(0, MAX_TAGS)
    .map(([token]) => token)
}

export const jaccard = (a: string[], b: string[]) => {
  const left = new Set(a)
  const right = new Set(b)
  const shared = [...left].filter(token => right.has(token)).length
  const union = left.size + right.size - shared
  return union === 0 ? 0 : shared / union
}

const bestMatch = (ledger: SkillLedger, tags: string[]) =>
  Object.values(ledger)
    .map(record => ({ record, overlap: jaccard(record.tags, tags) }))
    .filter(match => match.overlap >= MIN_JACCARD)
    .sort((a, b) => b.overlap - a.overlap)[0]?.record

const baseSlug = (tags: string[]) => tags.slice(0, MAX_SLUG_TAGS).join('-').slice(0, MAX_SLUG_CHARS).replace(/-+$/, '') || FALLBACK_SLUG

const uniqueSlug = (ledger: SkillLedger, tags: string[]) => {
  const base = baseSlug(tags)
  let slug = base
  for (let n = 2; ledger[slug]; n++) slug = `${base.slice(0, MAX_SLUG_CHARS - `-${n}`.length)}-${n}`
  return slug
}

const attach = (record: SkillRecord, signal: SkillSignal): SkillRecord => ({
  ...record,
  sessions: record.sessions.includes(signal.sessionId) ? record.sessions : [...record.sessions, signal.sessionId],
  signals: [...record.signals, signal].slice(-MAX_SIGNALS_PER_RECORD),
})

const candidate = (slug: string, tags: string[], signal: SkillSignal): SkillRecord =>
  attach(
    { slug, status: 'candidate', tags, sessions: [], signals: [], citedPaths: [], headCommit: '', createdAt: signal.at, idleSessions: 0, uses: 0, cleanUses: 0 },
    signal,
  )

export const groupSignal = (ledger: SkillLedger, signal: SkillSignal) => {
  const tags = signalTags(signal)
  const match = bestMatch(ledger, tags)
  if (match && BLOCKED_STATUSES.includes(match.status)) return { ledger, slug: match.slug }
  if (match) return { ledger: { ...ledger, [match.slug]: attach(match, signal) }, slug: match.slug }
  const slug = uniqueSlug(ledger, tags)
  return { ledger: { ...ledger, [slug]: candidate(slug, tags, signal) }, slug }
}

export const readyToPromote = (record: SkillRecord, activeCount: number) =>
  record.status === 'candidate' && new Set(record.sessions).size >= PROMOTE_MIN_SESSIONS && activeCount < MAX_ACTIVE_SKILLS

const setStatus = (ledger: SkillLedger, slug: string, status: SkillRecord['status']) =>
  ledger[slug] ? { ...ledger, [slug]: { ...ledger[slug], status } } : ledger

export const reject = (ledger: SkillLedger, slug: string) => setStatus(ledger, slug, 'rejected')

export const restore = (ledger: SkillLedger, slug: string) => setStatus(ledger, slug, 'candidate')
