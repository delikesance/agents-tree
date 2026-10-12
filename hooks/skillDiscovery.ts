import { tokenize } from './knowledge'
import { jaccard } from './skillLedger'
import type { SkillLedger } from './skillTypes'

export type DiscoveryCandidate = { slug: string; title: string; steps: string[]; files: string[] }
export type Criterion = 'multi-step' | 'grounded' | 'new' | 'not-in-context'
export type Verdict = { candidate: DiscoveryCandidate; failed: Criterion[]; covered: number }

const MIN_STEPS = 3
const MAX_SLUG_CHARS = 40
const MAX_OVERLAP = 0.4
export const CONTEXT_OVERLAP = 0.7
const SHINGLE_SIZE = 3
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

export const pathsIn = (steps: string[]) => [...new Set(steps.flatMap(spansOf).filter(span => PATH_LIKE.test(span)))]

export const candidateOf = (title: string, steps: string[]): DiscoveryCandidate => ({
  slug: slugOf(title),
  title,
  steps,
  files: pathsIn(steps),
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

const shinglesOf = (text: string) => {
  const tokens = tokenize(text)
  const size = Math.min(SHINGLE_SIZE, tokens.length)
  return Array.from({ length: Math.max(tokens.length - size + 1, 0) }, (_, index) => tokens.slice(index, index + size).join(' '))
}

const coveredSteps = (candidate: DiscoveryCandidate, context: string[]) => {
  const contextLines = context.flatMap(text => text.split('\n')).map(shinglesOf)
  const isCovered = (step: string) => {
    const shingles = shinglesOf(step)
    return contextLines.some(lines => jaccard(shingles, lines) >= CONTEXT_OVERLAP)
  }
  return candidate.steps.filter(isCovered).length
}

const criteria: Record<Criterion, (candidate: DiscoveryCandidate, ledger: SkillLedger, covered: number) => boolean> = {
  'multi-step': candidate => candidate.steps.length >= MIN_STEPS,
  grounded: candidate => candidate.files.length > 0,
  new: (candidate, ledger) => {
    const tokens = tokenize(candidate.title)
    return !ledger[candidate.slug] && Object.values(ledger).every(record => jaccard(record.tags, tokens) < MAX_OVERLAP)
  },
  'not-in-context': (candidate, _ledger, covered) => covered < candidate.steps.length,
}

export const judge = (candidate: DiscoveryCandidate, ledger: SkillLedger, context: string[] = []): Verdict => {
  const covered = coveredSteps(candidate, context)
  return {
    candidate,
    covered,
    failed: (Object.keys(criteria) as Criterion[]).filter(name => !criteria[name](candidate, ledger, covered)),
  }
}
