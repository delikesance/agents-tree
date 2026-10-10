export const STORE_KEY = 'knowledge'
const MAX_ENTRIES = 400
const MAX_RESULTS = 5
export const MAX_LEARNED_TAGS = 12
export const LEARNED_KEY_CHARS = 80
const SNIPPET_CHARS = 280
const TAG_WEIGHT = 3
const MIN_TOKEN_LENGTH = 3
export const KNOWLEDGE_SECTION = {
  id: 'agent-graph:knowledge',
  text: 'Knowledge cache: mcp__agent-graph__recall searches saved findings by keywords. After finding where something lives or how a problem was solved, call mcp__agent-graph__remember with a short factual entry (paths, symbols, conclusion).',
  scope: 'session',
} as const
export type Entry = { key: string; tags: string[]; text: string; at: number; learned?: true }

export const REMEMBER_TOOL = {
  name: 'remember',
  description:
    'Save a durable finding (where something lives in the codebase, a solution, a decision and its why) so any later session can recall it instead of re-exploring. Same key overwrites. Keep text short and factual.',
  inputSchema: {
    type: 'object',
    properties: {
      key: { type: 'string', description: 'Short unique slug, e.g. "auth-token-refresh-flow"' },
      text: { type: 'string', description: 'The finding: paths, symbols, conclusion' },
      tags: { type: 'array', items: { type: 'string' }, description: 'Search keywords' },
    },
    required: ['key', 'text'],
  },
} as const

export const RECALL_TOOL = {
  name: 'recall',
  description:
    'Search saved findings before exploring the codebase or re-solving a problem. Returns the best matches as short snippets; pass `key` to get one entry in full.',
  inputSchema: {
    type: 'object',
    properties: {
      query: { type: 'string', description: 'Keywords' },
      key: { type: 'string', description: 'Exact key to read in full' },
    },
  },
} as const

const STOPWORDS = new Set(['the', 'and', 'for', 'with', 'that', 'this', 'from', 'les', 'des', 'une', 'pour', 'dans', 'que', 'qui', 'est', 'sur', 'par', 'pas'])

const singular = (token: string) => (token.length > MIN_TOKEN_LENGTH && /[^s]s$/.test(token) ? token.slice(0, -1) : token)

export const tokenize = (value: string) =>
  value.toLowerCase().normalize('NFD').replace(/\p{M}/gu, '').split(/[^\p{L}\p{N}_]+/u).filter(token => token.length >= MIN_TOKEN_LENGTH && !STOPWORDS.has(token)).map(singular)

const score = (entry: Entry, queryTokens: string[]) => {
  const tagTokens = new Set(entry.tags.flatMap(tokenize).concat(tokenize(entry.key)))
  const textTokens = new Set(tokenize(entry.text))
  return queryTokens.reduce((sum, token) => sum + (tagTokens.has(token) ? TAG_WEIGHT : 0) + (textTokens.has(token) ? 1 : 0), 0)
}

export const clip = (text: string) => (text.length > SNIPPET_CHARS ? `${text.slice(0, SNIPPET_CHARS)}…` : text)

export const snippet = (entry: Entry) =>
  `[${entry.key}] ${entry.tags.join(',')} (${new Date(entry.at).toISOString().slice(0, 10)})\n${clip(entry.text)}`

export const rank = (entries: Entry[], query: string, limit: number) => {
  const queryTokens = tokenize(query)
  return entries
    .map(entry => ({ entry, points: score(entry, queryTokens) }))
    .filter(hit => hit.points > 0)
    .sort((a, b) => b.points - a.points || b.entry.at - a.entry.at)
    .slice(0, limit)
    .map(hit => hit.entry)
}

export const search = (entries: Entry[], query: string) => {
  const hits = rank(entries, query, MAX_RESULTS)
  return hits.length ? hits.map(snippet).join('\n\n') : 'Aucun résultat.'
}

export const upsert = (entries: Entry[], entry: Entry) => {
  const kept = [...entries.filter(existing => existing.key !== entry.key), entry]
  const evicted = new Set([...kept.filter(item => item.learned), ...kept.filter(item => !item.learned)].slice(0, Math.max(0, kept.length - MAX_ENTRIES)))
  return kept.filter(item => !evicted.has(item))
}
export const SECRET_PATTERN = /\b(sk-[\w-]{16,}|gh[pousr]_\w{20,}|AKIA\w{12,})\b|\b(api[_-]?key|token|secret|password)\s*[:=]\s*\S+/gi
export const SECRET_MASK = '[masqué]'
