import type { Group } from './mapGroups'
import { mergeSmallest } from './mapMerge'
import { MAX_SYMBOLS_PER_FILE, symbolEntry } from './mapSymbols'
import type { Symbols } from './mapSymbols'

const SYMBOL_LEVELS = [0, 1, 3, MAX_SYMBOLS_PER_FILE]
const MAX_LISTED_FILES = 8

const plural = (count: number, noun: string) => `${count} ${noun}${count === 1 ? '' : 's'}`

const countLabel = ({ files, tests }: Group) => [files.length && plural(files.length, 'file'), tests.length && plural(tests.length, 'test')].filter(Boolean).join(' + ')

const listed = (entries: string[]) => (entries.length > MAX_LISTED_FILES ? [...entries.slice(0, MAX_LISTED_FILES), `+${entries.length - MAX_LISTED_FILES}`] : entries)

const groupLine = (group: Group, summaries: Map<string, string>, symbols: Symbols, limit?: number) => {
  const purpose = summaries.get(group.name)
  const entries = limit === undefined ? [] : listed(group.files.flatMap(file => symbolEntry(file, symbols, limit) ?? []))
  return `- ${group.name} (${countLabel(group)})${purpose ? `: ${purpose}` : ''}${entries.length ? ` -> ${entries.join('; ')}` : ''}`
}

export const renderGroups = (groups: Group[], summaries: Map<string, string>, symbols: Symbols, charBudget: number) => {
  const render = (shown: Group[], levels?: number[]) => shown.map((group, i) => groupLine(group, summaries, symbols, levels && SYMBOL_LEVELS[levels[i] ?? 0]))
  const fits = (lines: string[]) => lines.join('\n').length <= charBudget

  const raiseLevels = (shown: Group[]) => {
    const levels = shown.map(() => 0)
    if (!fits(render(shown, levels))) return undefined
    const smallestFirst = shown.map((_, i) => i).sort((a, b) => (shown[a]?.files.length ?? 0) - (shown[b]?.files.length ?? 0))
    for (let level = 1; level < SYMBOL_LEVELS.length; level++) {
      for (const i of smallestFirst) {
        if (levels[i] !== level - 1) continue
        levels[i] = level
        if (!fits(render(shown, levels))) levels[i] = level - 1
      }
    }
    return render(shown, levels)
  }

  let shown = groups
  for (;;) {
    const raised = raiseLevels(shown)
    if (raised) return raised
    if (fits(render(shown))) return render(shown)
    const merged = mergeSmallest(shown)
    if (merged === shown) return render(shown)
    shown = merged
  }
}
