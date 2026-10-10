import { commitLine } from './mapState'

export const MAP_SUMMARY_MAX_TOKENS = 800

const ROOT_DIR = '.'
const MAX_DIRS = 40
const MAX_LISTED = 10
const SAMPLE_FILES = 8
const TOP_EXTENSIONS = 2
const SUMMARY_MAX = 120
const ENTRY_MAX_DEPTH = 3
const ENTRY_POINT = /(^|\/)(main|index|app|cli|register|lib|server|__main__|manage)\.\w+$/
const DOC_FILE = /(^|\/)(CLAUDE\.md|README(\.\w+)?)$/i

export type Directory = { name: string; files: string[] }

const dirOf = (file: string) => (file.includes('/') ? file.split('/', 1)[0] ?? ROOT_DIR : ROOT_DIR)

export const topDirectories = (files: string[]): Directory[] => {
  const byName = new Map<string, string[]>()
  for (const file of files) byName.set(dirOf(file), [...(byName.get(dirOf(file)) ?? []), file])
  return [...byName].map(([name, list]) => ({ name, files: list })).sort((a, b) => b.files.length - a.files.length).slice(0, MAX_DIRS)
}

export const SUMMARY_PROMPT = `For each directory below, write one line "<directory>: <what it contains, 12 words max>". Reply with those lines only, one per directory, nothing else.\n\n`

export const summaryInput = (dirs: Directory[]) => dirs.map(({ name, files }) => `${name}: ${files.slice(0, SAMPLE_FILES).join(', ')}`).join('\n')

export const parseSummaries = (text: string, dirs: Directory[]) => {
  const known = new Set(dirs.map(({ name }) => name))
  const summaries = new Map<string, string>()
  for (const line of text.split('\n')) {
    const split = line.indexOf(': ')
    const name = line.slice(0, split).replace(/^[-*\s`]+|[\s`/]+$/g, '')
    const summary = line.slice(split + 2).trim()
    if (split > 0 && summary && known.has(name)) summaries.set(name, summary.slice(0, SUMMARY_MAX))
  }
  return summaries
}

const extensionOf = (file: string) => /\.\w+$/.exec(file.split('/').pop() ?? '')?.[0]

const dominantExtensions = (files: string[]) => {
  const counts = new Map<string, number>()
  for (const extension of files.map(extensionOf)) if (extension) counts.set(extension, (counts.get(extension) ?? 0) + 1)
  return [...counts].sort((a, b) => b[1] - a[1]).slice(0, TOP_EXTENSIONS).map(([extension]) => extension)
}

const layoutLine = ({ name, files }: Directory, summaries: Map<string, string>) => {
  const extensions = dominantExtensions(files).join(', ')
  const summary = summaries.get(name)
  return `- ${name === ROOT_DIR ? 'root files' : `${name}/`} -> ${files.length} files${extensions && ` (${extensions})`}${summary ? `: ${summary}` : ''}`
}

const section = (title: string, lines: string[]) => (lines.length ? ['', `## ${title}`, ...lines] : [])

const listed = (files: string[], pattern: RegExp) => files.filter(file => pattern.test(file)).slice(0, MAX_LISTED).map(file => `- ${file}`)

export type MapInput = { head: string; files: string[]; testCommand?: string; summaries: Map<string, string> }

export const buildMap = ({ head, files, testCommand, summaries }: MapInput) =>
  [
    commitLine(head),
    '# Project map',
    ...section('Commands', testCommand ? [`- Test: \`${testCommand}\``] : []),
    ...section('Layout', topDirectories(files).map(dir => layoutLine(dir, summaries))),
    ...section('Entry points', listed(files.filter(file => file.split('/').length <= ENTRY_MAX_DEPTH), ENTRY_POINT)),
    ...section('Docs', listed(files, DOC_FILE)),
  ].join('\n') + '\n'
