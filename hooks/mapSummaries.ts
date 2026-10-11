import { baseName } from './mapGroups'
import type { Group } from './mapGroups'
import { symbolEntry } from './mapSymbols'
import type { Symbols } from './mapSymbols'

export const MAP_SUMMARY_MAX_TOKENS = 800

const SAMPLE_FILES = 8
const SUMMARY_MAX = 120

export const SUMMARY_PROMPT = `For each group below, write one line "<group>: <what it is for, 12 words max>". Reply with those lines only, one per group, nothing else. Describe only the files listed after each group name (test files count); never mention missing or stray files.\n\n`

export const summaryInput = (groups: Group[], symbols: Symbols) =>
  groups.map(({ name, files, tests }) => `${name}: ${(files.length ? files : tests).slice(0, SAMPLE_FILES).map(file => symbolEntry(file, symbols) ?? baseName(file)).join(', ')}`).join('\n')

const cleanName = (name: string) => name.replace(/^[-*\s`]+|[\s`/]+$/g, '')

export const parseSummaries = (text: string, groups: Group[]) => {
  const known = new Map(groups.map(({ name }) => [cleanName(name), name]))
  const summaries = new Map<string, string>()
  for (const line of text.split('\n')) {
    const split = line.indexOf(': ')
    const name = known.get(cleanName(line.slice(0, split)))
    const summary = line.slice(split + 2).trim()
    if (split > 0 && summary && name) summaries.set(name, summary.slice(0, SUMMARY_MAX))
  }
  return summaries
}
