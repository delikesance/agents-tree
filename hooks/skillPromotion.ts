import type { SkillDeps } from './skillHost'
import { synthesizeSkill } from './skillSynthesis'
import type { SkillRecord } from './skillTypes'
import { writeSkill } from './skillWriter'

const MAX_CITED_PATHS = 12

const relativeTo = (cwd: string) => (file: string) => {
  if (file.startsWith(`${cwd}/`)) return file.slice(cwd.length + 1)
  return file.startsWith('/') ? undefined : file
}

const citedPathsOf = (record: SkillRecord, cwd: string) => [
  ...new Set(record.signals.flatMap(signal => signal.files).map(relativeTo(cwd)).filter((file): file is string => !!file)),
].slice(0, MAX_CITED_PATHS)

export type PromotionPatch = Pick<SkillRecord, 'status' | 'citedPaths' | 'headCommit'>

export async function promotionPatch(deps: SkillDeps, cwd: string, record: SkillRecord): Promise<PromotionPatch | undefined> {
  const candidate = { ...record, citedPaths: citedPathsOf(record, cwd) }
  const synthesized = await synthesizeSkill(candidate, deps.fs)
  if (!synthesized) return undefined
  const written = await writeSkill(deps.fs, cwd, candidate, synthesized)
  return written.ok ? { status: 'active', citedPaths: written.citedPaths, headCommit: written.headCommit } : undefined
}
