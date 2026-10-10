export const MAP_FILE = '.claude/project-map.md'

export type Run = (argv: string[]) => Promise<{ exitCode: number; stdout: string }>
export type MapStatus = 'missing' | 'manual' | 'fresh' | 'stale'

const COMMIT_LINE = /^<!-- map: (\w+) -->/

export const commitLine = (sha: string) => `<!-- map: ${sha} -->`

const writtenCommit = (text: string) => COMMIT_LINE.exec(text.split('\n', 1)[0] ?? '')?.[1]

const changesStructure = (nameStatus: string) =>
  nameStatus.split('\n').some(line => line.split('\t').slice(1).some(path => path !== MAP_FILE))

export type MapReader = { run: Run; read: (path: string) => Promise<string>; exists: (path: string) => Promise<boolean> }

export const mapStatus = async ({ run, read, exists }: MapReader, head: string): Promise<MapStatus> => {
  if (!(await exists(MAP_FILE))) return 'missing'
  const text = await read(MAP_FILE).catch(() => undefined)
  const written = text === undefined ? undefined : writtenCommit(text)
  if (!written) return 'manual'
  if (written === head) return 'fresh'
  const diff = await run(['git', 'diff', '--name-status', '-M', '--diff-filter=ADR', written, head])
  return diff.exitCode !== 0 || changesStructure(diff.stdout) ? 'stale' : 'fresh'
}
