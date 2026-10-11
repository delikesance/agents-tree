import { expect, test } from 'claude-code/testing'
import { skillNudge } from './skillNudge'
import type { SkillLedger, SkillRecord } from './skillTypes'

const record = (slug: string, change: Partial<SkillRecord>): SkillRecord => ({ slug, status: 'active', tags: [], sessions: [], signals: [], citedPaths: [], headCommit: '', createdAt: 0, ...change }) as SkillRecord
const ledger = (...records: SkillRecord[]): SkillLedger => Object.fromEntries(records.map(r => [r.slug, r]))

test('suggests an active skill whose cited path is in the prompt', () => {
  expect(skillNudge(ledger(record('arch', { citedPaths: ['hooks/x.ts'] })), 'edit hooks/x.ts')).toContain('/arch')
})

test('suggests an active skill when its file basename is mentioned', () => {
  expect(skillNudge(ledger(record('arch', { citedPaths: ['hooks/x.ts'] })), 'fix x.ts')).toContain('/arch')
})

test('ignores unrelated prompts and non-active skills', () => {
  const skills = ledger(record('a', { citedPaths: ['hooks/x.ts'] }), record('b', { citedPaths: ['hooks/y.ts'], status: 'candidate' }))
  expect(skillNudge(skills, 'edit hooks/y.ts')).toBeUndefined()
  expect(skillNudge(skills, 'hello')).toBeUndefined()
})
