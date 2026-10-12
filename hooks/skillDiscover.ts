import type { SkillDeps } from './skillHost'
import { judge, markdownCandidates, type DiscoveryCandidate, type Verdict } from './skillDiscovery'
import { buildDigest } from './skillDigest'
import { proposeCandidates } from './skillPropose'
import { isLive } from './skillMaintenance'
import { promotionPatch } from './skillPromotion'
import { headCommit } from './skillWriter'
import { MAX_ACTIVE_SKILLS, SKILLS_STORE_KEY, type SkillLedger, type SkillRecord, type SkillSignal } from './skillTypes'

const SOURCES = ['README.md']
const PERMANENT_CONTEXT = ['CLAUDE.md', 'AGENTS.md', '.claude/project-map.md']

const groundedIn = async (deps: SkillDeps, cwd: string, candidate: DiscoveryCandidate): Promise<DiscoveryCandidate> => {
  const found = await Promise.all(candidate.files.map(async file => ((await deps.fs.read(`${cwd}/${file}`).catch(() => undefined)) === undefined ? undefined : file)))
  return { ...candidate, files: found.filter((file): file is string => !!file) }
}

const readAll = (deps: SkillDeps, cwd: string, sources: string[]) =>
  Promise.all(sources.map(source => deps.fs.read(`${cwd}/${source}`).catch(() => undefined)))

const repoRoot = (deps: SkillDeps, cwd: string) => deps.fs.run(['git', 'rev-parse', '--show-toplevel']).then(out => out.trim() || cwd).catch(() => cwd)

const PROPOSALS_KEY = `${SKILLS_STORE_KEY}.proposals`

type CachedProposals = { head: string; candidates: DiscoveryCandidate[] }

const cachedProposals = async (deps: SkillDeps, propose: () => Promise<DiscoveryCandidate[]>) => {
  const head = await headCommit(deps.fs)
  const cached = (await deps.store.get(PROPOSALS_KEY)) as CachedProposals | undefined
  if (head && cached?.head === head) return cached.candidates
  const candidates = await propose()
  if (head && candidates.length) await deps.store.set(PROPOSALS_KEY, { head, candidates } satisfies CachedProposals)
  return candidates
}

export async function discoverVerdicts(deps: SkillDeps, cwdOfSession: string, ledger: SkillLedger, signals: SkillSignal[]): Promise<Verdict[]> {
  const cwd = await repoRoot(deps, cwdOfSession)
  const texts = await readAll(deps, cwd, SOURCES)
  const context = (await readAll(deps, cwd, PERMANENT_CONTEXT)).filter((text): text is string => !!text)
  const documented = texts.flatMap(text => (text ? markdownCandidates(text) : []))
  const proposed = await cachedProposals(deps, async () => proposeCandidates(deps.fs, await buildDigest(deps, cwd, signals), context))
  const candidates = await Promise.all([...documented, ...proposed].map(candidate => groundedIn(deps, cwd, candidate)))
  return candidates.map(candidate => judge(candidate, ledger, context))
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
    const outcome = await promotionPatch(deps, cwd, record).catch(() => undefined)
    if (outcome?.ok) adopted = { ...adopted, [record.slug]: { ...record, ...outcome.value } }
  }
  return adopted
}

const reasonOf = ({ candidate, failed, covered }: Verdict) =>
  failed.includes('not-in-context')
    ? `déjà dans le contexte permanent (${covered}/${candidate.steps.length} étapes)`
    : failed.join(', ')

export const verdictLines = (verdicts: Verdict[]) =>
  verdicts.map(verdict => (verdict.failed.length ? `✘ ${verdict.candidate.slug} — ${reasonOf(verdict)}` : `✔ ${verdict.candidate.slug}`)).join('\n')
