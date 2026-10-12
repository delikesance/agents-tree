import type { SessionOutcome, SkillLedger, SkillRecord } from './skillTypes'

export const GAIN_MIN_USES = 3

const averageCost = ({ sessions, turns, corrections }: SessionOutcome) => (turns + corrections) / sessions

export const skillGain = (record: SkillRecord): number | undefined => {
  const withSkill = record.outcomes?.with
  const withoutSkill = record.outcomes?.without
  if (record.uses < GAIN_MIN_USES || !withSkill?.sessions || !withoutSkill?.sessions) return undefined
  return averageCost(withoutSkill) - averageCost(withSkill)
}

export const lowGainSlugs = (ledger: SkillLedger, minUses = GAIN_MIN_USES, maxGain = 0): string[] =>
  Object.values(ledger)
    .filter(record => record.status === 'active' && record.uses >= minUses)
    .filter(record => {
      const gain = skillGain(record)
      return gain !== undefined && gain <= maxGain
    })
    .map(record => record.slug)
