import { expect, test } from 'claude-code/testing'
import { lowGainSlugs, skillGain } from './skillMetric'
import type { SkillLedger, SkillRecord, SkillStatus } from './skillTypes'

const skill = (slug: string, overrides: Partial<SkillRecord> = {}): SkillRecord => ({
  slug,
  status: 'active',
  tags: [],
  sessions: [],
  signals: [],
  citedPaths: [],
  headCommit: 'abc',
  createdAt: 0,
  idleSessions: 0,
  uses: 3,
  cleanUses: 3,
  outcomes: {
    with: { sessions: 2, turns: 4, corrections: 0 },
    without: { sessions: 2, turns: 10, corrections: 0 },
  },
  ...overrides,
})

const ledgerOf = (...records: SkillRecord[]): SkillLedger =>
  Object.fromEntries(records.map(record => [record.slug, record]))

const losingSkill = (slug: string, status: SkillStatus): SkillRecord =>
  skill(slug, {
    status,
    outcomes: {
      with: { sessions: 2, turns: 12, corrections: 0 },
      without: { sessions: 2, turns: 4, corrections: 0 },
    },
  })

test('skillGain is undefined without outcomes on either side', () => {
  expect(skillGain(skill('bare', { outcomes: undefined }))).toBeUndefined()
  expect(
    skillGain(skill('one-sided', { outcomes: { with: { sessions: 0, turns: 0, corrections: 0 }, without: { sessions: 2, turns: 4, corrections: 0 } } })),
  ).toBeUndefined()
})

test('skillGain is positive when the skill lowers the average cost', () => {
  expect(skillGain(skill('helpful'))).toBe(3)
})

test('lowGainSlugs flags zero and negative gain but not positive gain', () => {
  const zero = skill('zero', { outcomes: { with: { sessions: 2, turns: 6, corrections: 0 }, without: { sessions: 2, turns: 6, corrections: 0 } } })
  const negative = skill('negative', { outcomes: { with: { sessions: 2, turns: 12, corrections: 0 }, without: { sessions: 2, turns: 4, corrections: 0 } } })
  const positive = skill('positive')
  expect(lowGainSlugs(ledgerOf(zero, negative, positive)).sort()).toEqual(['negative', 'zero'])
})

test('lowGainSlugs skips inactive, stale and archived skills', () => {
  const ledger = ledgerOf(
    losingSkill('stale-skill', 'stale'),
    losingSkill('archived-skill', 'archived'),
    losingSkill('candidate-skill', 'candidate'),
    losingSkill('active-skill', 'active'),
  )
  expect(lowGainSlugs(ledger)).toEqual(['active-skill'])
})

test('lowGainSlugs on an empty ledger is empty', () => {
  expect(lowGainSlugs({})).toEqual([])
})
