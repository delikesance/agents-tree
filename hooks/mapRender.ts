import { groupSize, mergeSmallest } from './mapGroups'
import type { Group } from './mapGroups'
import { symbolEntry } from './mapSymbols'
import type { Symbols } from './mapSymbols'

const countLabel = ({ files, tests }: Group) => [files.length && `${files.length} files`, tests.length && `${tests.length} tests`].filter(Boolean).join(' + ')

const groupLine = (group: Group, summaries: Map<string, string>, symbols?: Symbols) => {
  const purpose = summaries.get(group.name)
  const entries = symbols ? group.files.flatMap(file => symbolEntry(file, symbols) ?? []) : []
  const name = group.merged ? `${group.name} +${group.merged} more` : group.name
  return `- ${name} (${countLabel(group)})${purpose ? `: ${purpose}` : ''}${entries.length ? ` -> ${entries.join('; ')}` : ''}`
}

const largest = (groups: Group[]) => [...groups].sort((a, b) => groupSize(b) - groupSize(a))[0]

export const renderGroups = (groups: Group[], summaries: Map<string, string>, symbols: Symbols, charBudget: number) => {
  const withoutSymbols = new Set<string>()
  let shown = groups
  const render = () => shown.map(group => groupLine(group, summaries, withoutSymbols.has(group.name) ? undefined : symbols))
  while (render().join('\n').length > charBudget) {
    const widest = largest(shown.filter(group => !withoutSymbols.has(group.name)))
    if (widest) withoutSymbols.add(widest.name)
    else if (shown.length > 1) shown = mergeSmallest(shown)
    else break
  }
  return render()
}
