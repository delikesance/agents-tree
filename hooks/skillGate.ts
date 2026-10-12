import type { SkillLedger } from './skillTypes'

export type GateVerdict = { deny: string; slug: string }

export const relativeTo = (cwd: string, file: string) => (file.startsWith(`${cwd}/`) ? file.slice(cwd.length + 1) : undefined)

export const covers =(cited: string, file: string) => file === cited || file.startsWith(`${cited.replace(/\/+$/, '')}/`)

export const skillGate = (ledger: SkillLedger, cwd: string, files: string[], invoked: Set<string>, denied: Set<string>): GateVerdict | undefined => {
  const touched = files.map(file => relativeTo(cwd, file)).filter((file): file is string => !!file)
  const record = Object.values(ledger).find(
    candidate => candidate.status === 'active' && !invoked.has(candidate.slug) && !denied.has(candidate.slug) && candidate.citedPaths.some(cited => touched.some(file => covers(cited, file))),
  )
  return record && { slug: record.slug, deny: `Learned skill /${record.slug} covers this file: invoke it with the Skill tool first, then retry.` }
}
