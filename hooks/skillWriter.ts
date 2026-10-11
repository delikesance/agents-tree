import { renderSkill, validateSkill } from './skillRender'
import type { SynthesizedSkill } from './skillSynthesis'
import { SKILLS_DIR, SKILL_FILE, type SkillFs, type SkillRecord } from './skillTypes'

export type WriteResult = { ok: true; citedPaths: string[]; headCommit: string } | { ok: false; errors: string[] }

const isSafeSlug = (slug: string) => /^[a-z0-9][a-z0-9-]*$/.test(slug)

export const skillPath = (cwd: string, slug: string) => `${cwd}/${SKILLS_DIR}/${slug}/${SKILL_FILE}`

async function existingPaths(fs: SkillFs, cwd: string, paths: string[]): Promise<string[]> {
  const found = await Promise.all(paths.map(async path => ((await fs.read(`${cwd}/${path}`).catch(() => undefined)) === undefined ? undefined : path)))
  return found.filter((path): path is string => path !== undefined)
}

const headCommit = async (fs: SkillFs) => (await fs.run(['git', 'rev-parse', 'HEAD']).catch(() => '')).trim()

export async function writeSkill(fs: SkillFs, cwd: string, record: SkillRecord, synthesized: SynthesizedSkill): Promise<WriteResult> {
  if (!isSafeSlug(record.slug)) return { ok: false, errors: ['invalid slug'] }
  const citedPaths = await existingPaths(fs, cwd, record.citedPaths)
  const text = renderSkill({ slug: record.slug, ...synthesized, citedPaths })
  const errors = validateSkill(text)
  if (errors.length) return { ok: false, errors }
  await fs.write(skillPath(cwd, record.slug), text)
  return { ok: true, citedPaths, headCommit: await headCommit(fs) }
}

export async function removeSkill(fs: SkillFs, cwd: string, slug: string): Promise<boolean> {
  if (!isSafeSlug(slug)) return false
  await fs.remove(skillPath(cwd, slug))
  return true
}
