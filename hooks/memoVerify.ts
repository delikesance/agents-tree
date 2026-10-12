import type { Anchor, Entry } from './knowledge'

export type AnchorDeps = {
  exists: (path: string) => Promise<boolean>
  read: (path: string) => Promise<string | undefined>
}

const anchorHolds = async ({ path, symbol }: Anchor, deps: AnchorDeps) => {
  if (!(await deps.exists(path))) return false
  if (symbol === undefined) return true
  return (await deps.read(path))?.includes(symbol) ?? false
}

export const verifyAnchors = async (entry: Entry, deps: AnchorDeps): Promise<'ok' | 'stale'> => {
  const holds = await Promise.all((entry.anchors ?? []).map(anchor => anchorHolds(anchor, deps)))
  return holds.every(Boolean) ? 'ok' : 'stale'
}

export const pruneStale = async (entries: Entry[], deps: AnchorDeps) => {
  const verdicts = await Promise.all(entries.map(entry => verifyAnchors(entry, deps)))
  return entries.filter((_, index) => verdicts[index] === 'ok')
}
