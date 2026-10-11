import type { SkillLedger } from './skillTypes'

export const skillNudge = (ledger: SkillLedger, prompt: string): string | undefined => {
  const slugs = Object.values(ledger)
    .filter(record => record.status === 'active' && record.citedPaths.some(path => prompt.includes(path)))
    .map(record => `/${record.slug}`)
  return slugs.length ? `Learned skill relevant to this prompt: invoke ${slugs.join(', ')} before starting.` : undefined
}
