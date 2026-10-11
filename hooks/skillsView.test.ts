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

test('summary lists non-empty counts', () => {
  expect(skillsSummary(ledger(record('a', 'active'), record('b', 'active'), record('c', 'candidate'), record('d', 'rejected')))).toBe('2 active · 1 candidates · 1 rejected')
  expect(skillsSummary({})).toBe('')
})
