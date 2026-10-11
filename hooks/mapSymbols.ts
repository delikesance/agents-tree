import { baseName } from './mapGroups'

export type Symbols = Map<string, string[]>

const MAX_READ_FILES = 150
const MAX_SYMBOLS_PER_FILE = 6

const SYMBOL_PATTERNS: Array<[RegExp, RegExp]> = [
  [/\.[cm]?[jt]sx?$/, /^export\s+(?:async\s+)?(?:const|function|type|class|interface|enum)\s+(\w+)/gm],
  [/\.rs$/, /^pub(?:\([\w:\s]+\))?\s+(?:async\s+)?(?:fn|struct|enum|trait|mod)\s+(\w+)/gm],
  [/\.py$/, /^(?:async\s+)?(?:def|class)\s+([A-Za-z]\w*)/gm],
]

const patternOf = (file: string) => SYMBOL_PATTERNS.find(([extension]) => extension.test(file))?.[1]

export const extractSymbols = (file: string, text: string) => {
  const pattern = patternOf(file)
  return pattern ? [...new Set([...text.matchAll(pattern)].flatMap(match => match[1] ?? []))] : []
}

export const readSymbols = async (read: (path: string) => Promise<string>, files: string[]): Promise<Symbols> => {
  const readable = files.filter(file => patternOf(file)).slice(0, MAX_READ_FILES)
  const entries = await Promise.all(readable.map(async file => [file, extractSymbols(file, await read(file).catch(() => ''))] as const))
  return new Map(entries.filter(([, symbols]) => symbols.length > 0))
}

export const symbolEntry = (file: string, symbols: Symbols) => {
  const list = symbols.get(file)
  if (!list) return undefined
  const hidden = list.length - MAX_SYMBOLS_PER_FILE
  return `${baseName(file)}(${[...list.slice(0, MAX_SYMBOLS_PER_FILE), ...(hidden > 0 ? [`+${hidden}`] : [])].join(', ')})`
}
