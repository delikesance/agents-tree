import type { SkillFs } from './skillTypes'

const HISTORY_COMMITS = 300
const MAX_FILES_PER_COMMIT = 12
const MIN_COCHANGES = 3
const MIN_COMPANION_SHARE = 0.6
const MAX_MINED = 5
const COMMIT_MARK = '@@'
const NOISE = /(^|\/)(\.claude|\.claudework|node_modules)\/|\.(lock|md|json)$/

type Commit = string[]

export const parseHistory = (log: string): Commit[] =>
  log
    .split(COMMIT_MARK)
    .map(block => block.split('\n').slice(1).map(line => line.trim()).filter(line => line && !NOISE.test(line)))
    .filter(files => files.length > 1 && files.length <= MAX_FILES_PER_COMMIT)

const countBy = (commits: Commit[]) => {
  const counts = new Map<string, number>()
  for (const file of commits.flat()) counts.set(file, (counts.get(file) ?? 0) + 1)
  return counts
}

const clusterOf = (hot: string, commits: Commit[]) => {
  const own = commits.filter(files => files.includes(hot))
  const companions = [...countBy(own)].filter(([file, count]) => file !== hot && count >= MIN_COCHANGES && count / own.length >= MIN_COMPANION_SHARE)
  return { files: [hot, ...companions.map(([file]) => file)], count: own.length }
}

export const coChangeClusters = (commits: Commit[]) => {
  const seen = new Set<string>()
  return [...countBy(commits)]
    .filter(([, count]) => count >= MIN_COCHANGES)
    .map(([hot]) => clusterOf(hot, commits))
    .filter(cluster => cluster.files.length > 1)
    .sort((a, b) => b.count - a.count)
    .filter(({ files }) => {
      const key = [...files].sort().join('|')
      return seen.has(key) ? false : (seen.add(key), true)
    })
    .slice(0, MAX_MINED)
}

export const readHistory = (fs: SkillFs) =>
  fs.run(['git', 'log', `-n${HISTORY_COMMITS}`, '--no-merges', '--name-only', `--pretty=format:${COMMIT_MARK}%s`]).then(parseHistory).catch((): Commit[] => [])
