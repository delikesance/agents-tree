import { expect, test } from 'claude-code/testing'
import { skillGate } from './skillGate'
import type { SkillLedger, SkillRecord } from './skillTypes'

const record = (overrides: Partial<SkillRecord> = {}): SkillRecord => ({
  slug: 'fix-api', status: 'active', tags: [], sessions: [], signals: [], citedPaths: ['src/api'], headCommit: '', createdAt: 0, idleSessions: 0, uses: 0, cleanUses: 0, ...overrides,
})
const ledgerOf = (overrides?: Partial<SkillRecord>): SkillLedger => ({ 'fix-api': record(overrides) })
const gate = (ledger: SkillLedger, file: string, invoked = new Set<string>(), denied = new Set<string>()) => skillGate(ledger, '/p', [file], invoked, denied)

test('skillGate: denies a write under a cited path', () => {
  expect(gate(ledgerOf(), '/p/src/api/x.ts')?.slug).toBe('fix-api')
})

test('skillGate: ignores unrelated files and files outside the project', () => {
  expect(gate(ledgerOf(), '/p/src/other.ts')).toBeUndefined()
  expect(gate(ledgerOf(), '/elsewhere/src/api/x.ts')).toBeUndefined()
})

test('skillGate: lets through an invoked or already denied skill', () => {
  expect(gate(ledgerOf(), '/p/src/api/x.ts', new Set(['fix-api']))).toBeUndefined()
  expect(gate(ledgerOf(), '/p/src/api/x.ts', new Set(), new Set(['fix-api']))).toBeUndefined()
})

test('skillGate: ignores inactive skills', () => {
  expect(gate(ledgerOf({ status: 'stale' }), '/p/src/api/x.ts')).toBeUndefined()
})
