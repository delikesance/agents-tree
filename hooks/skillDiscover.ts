import type { SkillDeps } from './skillHost'
import { judge, markdownCandidates, type DiscoveryCandidate, type Verdict } from './skillDiscovery'
import { isLive } from './skillMaintenance'
import { promotionPatch } from './skillPromotion'
import { MAX_ACTIVE_SKILLS, type SkillLedger, type SkillRecord } from './skillTypes'

const SOURCES = ['CLAUDE.md', 'README.md']

const groundedIn = async (deps: SkillDeps, cwd: string, candidate: DiscoveryCandidate): Promise<DiscoveryCandidate> => {
  const found = await Promise.all(candidate.files.map(async file => ((await deps.fs.read(`${cwd}/${file}`).catch(() => undefined)) === undefined ? undefined : file)))
  return { ...candidate, files: found.filter((file): file is string => !!file) }
}

export async function discoverVerdicts(deps: SkillDeps, cwd: string, ledger: SkillLedger): Promise<Verdict[]> {
  const texts = await Promise.all(SOURCES.map(source => deps.fs.read(`${cwd}/${source}`).catch(() => undefined)))
  const candidates = await Promise.all(texts.flatMap(text => (text ? markdownCandidates(text) : [])).map(candidate => groundedIn(deps, cwd, candidate)))
  return candidates.map(candidate => judge(candidate, ledger))
}

export const recordOf = (candidate: DiscoveryCandidate, at: number): SkillRecord => ({
  slug: candidate.slug,
  status: 'candidate',
  tags: candidate.title.toLowerCase().split(/[^a-z0-9]+/).filter(Boolean),
  sessions: [],
  signals: candidate.steps.map(step => ({ kind: 'discovery', sessionId: '', at, files: candidate.files, tools: [], note: `${candidate.title}: ${step}` })),
  citedPaths: candidate.files,
  headCommit: '',
  createdAt: at,
  idleSessions: 0,
  uses: 0,
  cleanUses: 0,
})

export async function adoptVerdicts(deps: SkillDeps, cwd: string, ledger: SkillLedger, verdicts: Verdict[]): Promise<SkillLedger> {
  const room = MAX_ACTIVE_SKILLS - Object.values(ledger).filter(isLive).length
  const accepted = verdicts.filter(verdict => !verdict.failed.length).slice(0, Math.max(room, 0))
  let adopted = ledger
  for (const { candidate } of accepted) {
    const record = recordOf(candidate, deps.now())
    const patch = await promotionPatch(deps, cwd, record).catch(() => undefined)
    if (patch) adopted = { ...adopted, [record.slug]: { ...record, ...patch } }
  }
  return adopted
}

export const verdictLines = (verdicts: Verdict[]) =>
  verdicts.map(({ candidate, failed }) => (failed.length ? `✘ ${candidate.slug} — ${failed.join(', ')}` : `✔ ${candidate.slug}`)).join('\n')
