import { entryPoints } from './mapEntries'
import type { Group } from './mapGroups'
import { renderGroups } from './mapRender'
import { commitLine } from './mapState'
import type { Symbols } from './mapSymbols'

const MAX_MAP_CHARS = 6_000

const section = (title: string, lines: string[]) => (lines.length ? ['', `## ${title}`, ...lines] : [])

export type MapInput = {
  head: string
  files: string[]
  groups: Group[]
  testCommand?: string
  summaries: Map<string, string>
  symbols: Symbols
  manual: string
}

export const buildMap = ({ head, files, groups, testCommand, summaries, symbols, manual }: MapInput) => {
  const lead = [
    commitLine(head),
    '# Project map',
    ...section('Commands', testCommand ? [`- Test: \`${testCommand}\``] : []),
    ...section('Entry points', entryPoints(files).map(file => `- ${file}`)),
  ]
  const charBudget = MAX_MAP_CHARS - lead.join('\n').length
  return [...lead, ...section('Where to look', renderGroups(groups, summaries, symbols, charBudget))].join('\n') + `\n\n${manual}`
}
