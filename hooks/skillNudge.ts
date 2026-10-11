import type { SkillLedger } from './skillTypes'

const basename = (path: string) => path.split('/').pop() ?? path

export const skillNudge = (ledger: SkillLedger, prompt: string): string | undefined => {
  const slugs = Object.values(ledger)
    .filter(record => record.status === 'active' && record.citedPaths.some(path => prompt.includes(path) || prompt.includes(basename(path))))
    .map(record => `/${record.slug}`)
  return slugs.length ? `Learned skill relevant to this prompt: invoke ${slugs.join(', ')} before starting.` : undefined
}
