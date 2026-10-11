import { test, expect } from 'claude-code/testing'
import { skillsSummary, viewSkills } from './skillsView'
import type { SkillLedger, SkillRecord, SkillStatus } from './skillTypes'

const record = (slug: string, status: SkillStatus, lastUsedAt?: number): SkillRecord => ({
  slug, status, tags: [], sessions: [], signals: [], citedPaths: [], headCommit: '', createdAt: 0, lastUsedAt, idleSessions: 0, uses: 2, cleanUses: 1,
})
const ledger = (...records: SkillRecord[]): SkillLedger => Object.fromEntries(records.map(item => [item.slug, item]))

test('rows are ordered stale, candidate, active by recent use, then closed', () => {
  const rows = viewSkills(ledger(record('old', 'active', 1), record('gone', 'archived'), record('new', 'active', 9), record('c', 'candidate'), record('s', 'stale')))
  expect(rows.map(row => row.slug)).toEqual(['s', 'c', 'new', 'old', 'gone'])
})

test('row summary prefers the first signal note, collapsed and truncated to 80 chars', () => {
  const note = `  first\n\tline   ${'x'.repeat(100)}`
  const withNote = { ...record('n', 'active'), signals: [{ kind: 'retry' as const, sessionId: 's', at: 1, files: [], tools: [], note }] }
  const [row] = viewSkills(ledger(withNote))
  expect(row.summary).toBe(`first line ${'x'.repeat(68)}…`)
  expect(row.summary.length).toBe(80)
})

test('summary lists non-empty counts', () => {
  expect(skillsSummary(ledger(record('a', 'active'), record('b', 'active'), record('c', 'candidate'), record('d', 'rejected')))).toBe('2 active · 1 candidates · 1 rejected')
  expect(skillsSummary({})).toBe('')
})
