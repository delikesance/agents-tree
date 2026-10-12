import { skillPath } from './skillWriter'
import { SKILLS_DIR, type SkillFs, type SkillLedger, type SkillRecord } from './skillTypes'

const DESCRIPTION_LINE = /^description:\s*(.+)$/m

const isLive = (record: SkillRecord) => record.status === 'active' || record.status === 'stale'

const installedSlugs = async (fs: SkillFs, cwd: string): Promise<string[]> =>
  (await fs.run(['ls', '-1', `${cwd}/${SKILLS_DIR}`]).catch(() => '')).split('\n').filter(Boolean)

const installedRecord = (slug: string, description?: string): SkillRecord => ({
  slug, status: 'active', tags: [], sessions: [], signals: [], citedPaths: [], headCommit: '',
  summary: description, createdAt: 0, idleSessions: 0, uses: 0, cleanUses: 0,
})

export async function withInstalledSkills(fs: SkillFs, cwd: string | undefined, ledger: SkillLedger): Promise<SkillLedger> {
  if (!cwd) return ledger
  const installed = new Set(await installedSlugs(fs, cwd))
  const kept = Object.fromEntries(Object.entries(ledger).filter(([slug, record]) => !isLive(record) || installed.has(slug)))
  const missing = [...installed].filter(slug => !kept[slug] || !isLive(kept[slug]))
  const added = await Promise.all(missing.map(async slug => {
    const description = DESCRIPTION_LINE.exec((await fs.read(skillPath(cwd, slug)).catch(() => undefined)) ?? '')?.[1]
    return [slug, kept[slug] ? { ...kept[slug], status: 'active' as const, summary: description ?? kept[slug].summary } : installedRecord(slug, description)] as const
  }))
  return { ...kept, ...Object.fromEntries(added) }
}
