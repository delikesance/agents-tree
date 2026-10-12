import { expect, test } from 'claude-code/testing'
import { lowGainSlugs, skillGain } from './skillMetric'
import type { SkillRecord } from './skillTypes'

const record = (overrides: Partial<SkillRecord> = {}): SkillRecord => ({
  slug: 'demo',
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
    with: { sessions: 3, turns: 6, corrections: 0 },
    without: { sessions: 2, turns: 20, corrections: 4 },
  },
  ...overrides,
})

test('skillGain is the average cost without the skill minus with it', () => {
  expect(skillGain(record())).toBe(10)
})

test('skillGain is undefined below the minimum uses', () => {
  expect(skillGain(record({ uses: 2 }))).toBe(undefined)
})

test('skillGain is undefined without a baseline', () => {
  const outcomes = { with: { sessions: 3, turns: 6, corrections: 0 }, without: { sessions: 0, turns: 0, corrections: 0 } }
  expect(skillGain(record({ outcomes }))).toBe(undefined)
})

test('lowGainSlugs keeps active measured skills at or below the gain threshold', () => {
  const helpful = record({ slug: 'helpful' })
  const harmful = record({
    slug: 'harmful',
    outcomes: { with: { sessions: 3, turns: 30, corrections: 0 }, without: { sessions: 3, turns: 6, corrections: 0 } },
  })
  const archived = record({ slug: 'archived', status: 'archived', outcomes: harmful.outcomes })
  const unmeasured = record({ slug: 'unmeasured', uses: 1 })
  expect(lowGainSlugs({ helpful, harmful, archived, unmeasured })).toEqual(['harmful'])
})
