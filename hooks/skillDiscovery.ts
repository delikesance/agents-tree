import { tokenize } from './knowledge'
import { jaccard } from './skillLedger'
import type { SkillLedger } from './skillTypes'

export type DiscoveryCandidate = { slug: string; title: string; steps: string[]; files: string[]; commands: string[] }
export type Criterion = 'multi-step' | 'grounded' | 'verifiable' | 'new'
export type Verdict = { candidate: DiscoveryCandidate; failed: Criterion[] }

const MIN_STEPS = 3
const MAX_SLUG_CHARS = 40
const MAX_OVERLAP = 0.4
const RUNNERS = ['bun', 'npm', 'pnpm', 'yarn', 'make', 'python3', 'git', 'cargo', 'go', 'pytest', 'tsc']
const HEADING = /^#{1,3}\s+(.+)/
const ITEM = /^\s*(?:[-*]|\d+\.)\s+(.+)/
const CODE_SPAN = /`([^`]+)`/g
const PATH_LIKE = /^[\w.@-]+(?:\/[\w.@-]+)*\.\w+$|^[\w.@-]+\/[\w./@-]*$/

const slugOf = (title: string) =>
  title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, MAX_SLUG_CHARS)
    .replace(/-+$/, '')

const spansOf = (text: string) => [...text.matchAll(CODE_SPAN)].map(match => match[1] ?? '')

const isCommand = (span: string) => span.includes(' ') && RUNNERS.includes(span.split(' ')[0] ?? '')

export const pathsIn = (steps: string[]) => [...new Set(steps.flatMap(spansOf).filter(span => PATH_LIKE.test(span)))]

const candidateOf = (title: string, steps: string[]): DiscoveryCandidate => ({
  slug: slugOf(title),
  title,
  steps,
  files: pathsIn(steps),
  commands: steps.flatMap(spansOf).filter(isCommand),
})

export function markdownCandidates(text: string): DiscoveryCandidate[] {
  const sections: { title: string; steps: string[] }[] = []
  for (const line of text.split('\n')) {
    const heading = line.match(HEADING)?.[1]
    if (heading) sections.push({ title: heading.trim(), steps: [] })
    const item = line.match(ITEM)?.[1]
    if (item) sections.at(-1)?.steps.push(item.trim())
  }
  return sections.filter(section => section.steps.length).map(section => candidateOf(section.title, section.steps))
}

const criteria: Record<Criterion, (candidate: DiscoveryCandidate, ledger: SkillLedger) => boolean> = {
  'multi-step': candidate => candidate.steps.length >= MIN_STEPS,
  grounded: candidate => candidate.files.length > 0,
  verifiable: candidate => candidate.commands.length > 0,
  new: (candidate, ledger) => {
    const tokens = tokenize(candidate.title)
    return !ledger[candidate.slug] && Object.values(ledger).every(record => jaccard(record.tags, tokens) < MAX_OVERLAP)
  },
}

export const judge = (candidate: DiscoveryCandidate, ledger: SkillLedger): Verdict => ({
  candidate,
  failed: (Object.keys(criteria) as Criterion[]).filter(name => !criteria[name](candidate, ledger)),
})
