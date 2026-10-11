import type { SkillDeps } from './skillHost'
import { endSessionTick, markStale, underperforming } from './skillLifecycle'
import type { SkillLedger, SkillRecord } from './skillTypes'
import { removeSkill } from './skillWriter'

export const isLive = (record: SkillRecord) => record.status === 'active' || record.status === 'stale'

const archiveWeak = (ledger: SkillLedger): SkillLedger =>
  Object.fromEntries(Object.entries(ledger).map(([slug, record]) => [slug, record.status === 'active' && underperforming(record) ? { ...record, status: 'archived' as const } : record]))

const newlyArchived = (before: SkillLedger, after: SkillLedger) =>
  Object.keys(after).filter(slug => after[slug]?.status === 'archived' && !!before[slug] && isLive(before[slug]))

export async function closeSession(deps: SkillDeps, ledger: SkillLedger, sessionId: string): Promise<SkillLedger> {
  const closed = archiveWeak(endSessionTick(ledger, sessionId))
  const { cwd } = deps
  if (cwd) await Promise.all(newlyArchived(ledger, closed).map(slug => removeSkill(deps.fs, cwd, slug).catch(() => false)))
  return closed
}

const changedSince = async (deps: SkillDeps, head: string) =>
  (await deps.fs.run(['git', 'diff', '--name-only', `${head}..HEAD`]).catch(() => '')).split('\n').filter(Boolean)

export async function flagStale(deps: SkillDeps, ledger: SkillLedger): Promise<SkillLedger> {
  const heads = new Set(Object.values(ledger).filter(record => record.status === 'active' && record.headCommit).map(record => record.headCommit))
  let flagged = ledger
  for (const head of heads) {
    const changed = await changedSince(deps, head)
    const group = Object.fromEntries(Object.entries(flagged).filter(([, record]) => record.headCommit === head))
    if (changed.length) flagged = { ...flagged, ...markStale(group, changed) }
  }
  return flagged
}
