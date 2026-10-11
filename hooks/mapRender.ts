import type { Group } from './mapGroups'
import { mergeSmallest } from './mapMerge'
import { MAX_SYMBOLS_PER_FILE, symbolEntry } from './mapSymbols'
import type { Symbols } from './mapSymbols'

const SYMBOL_LIMITS = [MAX_SYMBOLS_PER_FILE, 3, 1, 0]

const plural = (count: number, noun: string) => `${count} ${noun}${count === 1 ? '' : 's'}`

const countLabel = ({ files, tests }: Group) => [files.length && plural(files.length, 'file'), tests.length && plural(tests.length, 'test')].filter(Boolean).join(' + ')

const groupLine = (group: Group, summaries: Map<string, string>, symbols: Symbols, limit?: number) => {
  const purpose = summaries.get(group.name)
  const entries = limit === undefined ? [] : group.files.flatMap(file => symbolEntry(file, symbols, limit) ?? [])
  return `- ${group.name} (${countLabel(group)})${purpose ? `: ${purpose}` : ''}${entries.length ? ` -> ${entries.join('; ')}` : ''}`
}

export const renderGroups = (groups: Group[], summaries: Map<string, string>, symbols: Symbols, charBudget: number) => {
  const render = (shown: Group[], limit?: number) => shown.map(group => groupLine(group, summaries, symbols, limit))
  const fits = (lines: string[]) => lines.join('\n').length <= charBudget
  const withSymbols = SYMBOL_LIMITS.map(limit => render(groups, limit)).find(fits)
  if (withSymbols) return withSymbols
  let shown = groups
  while (!fits(render(shown))) {
    const merged = mergeSmallest(shown)
    if (merged === shown) break
    shown = merged
  }
  return render(shown)
}
