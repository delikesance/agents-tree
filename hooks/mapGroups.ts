export type Group = { name: string; files: string[]; tests: string[]; merged: number }

export const MAX_GROUPS = 40
const SPLIT_ABOVE = 15
const MIN_SPLIT_DEPTH = 2
const MAX_SPLIT_DEPTH = 4
const OTHER = 'other'
const TEST_FILE = /\.(test|spec)\.\w+$|_test\.\w+$|(^|\/)tests?\//
const FIRST_WORD = /^[A-Z]?[a-z0-9]+/

export const baseName = (file: string) => file.slice(file.lastIndexOf('/') + 1)

export const groupSize = ({ files, tests }: Group) => files.length + tests.length

const makeGroup = (name: string, all: string[]): Group => ({
  name,
  files: all.filter(file => !TEST_FILE.test(file)),
  tests: all.filter(file => TEST_FILE.test(file)),
  merged: 0,
})

const bucket = (files: string[], keyOf: (file: string) => string) => {
  const buckets = new Map<string, string[]>()
  for (const file of files) {
    const key = keyOf(file)
    const list = buckets.get(key) ?? []
    list.push(file)
    buckets.set(key, list)
  }
  return buckets
}

const prefixOf = (file: string) => FIRST_WORD.exec(baseName(file).split('.')[0] ?? '')?.[0]?.toLowerCase() ?? OTHER

const prefixGroups = (files: string[], dir: string) => {
  const label = dir || '.'
  const clusters = [...bucket(files, prefixOf)]
  const isCluster = ([key, list]: [string, string[]]) => key !== OTHER && list.length > 1
  const leftovers = clusters.filter(cluster => !isCluster(cluster)).flatMap(([, list]) => list)
  return [
    ...clusters.filter(isCluster).map(([key, list]) => makeGroup(`${label}/${key}*`, list)),
    ...(leftovers.length ? [makeGroup(`${label}/${OTHER}`, leftovers)] : []),
  ]
}

const relative = (file: string, dir: string) => (dir ? file.slice(dir.length + 1) : file)

const splitDir = (files: string[], dir: string, depth: number): Group[] => {
  if (depth >= MAX_SPLIT_DEPTH || (depth >= MIN_SPLIT_DEPTH && files.length <= SPLIT_ABOVE)) return [makeGroup(`${dir}/`, files)]
  const isNested = (file: string) => relative(file, dir).includes('/')
  const nested = bucket(files.filter(isNested), file => relative(file, dir).split('/')[0] ?? '')
  const direct = files.filter(file => !isNested(file))
  const label = `${dir || '.'}/${nested.size ? '*' : ''}`
  const own = direct.length > SPLIT_ABOVE ? prefixGroups(direct, dir) : direct.length ? [makeGroup(label, direct)] : []
  return [...own, ...[...nested].flatMap(([name, list]) => splitDir(list, dir ? `${dir}/${name}` : name, depth + 1))]
}

export const mergeSmallest = (groups: Group[]) => {
  const [smallest, next] = [...groups].sort((a, b) => groupSize(a) - groupSize(b))
  if (!smallest || !next) return groups
  const merged = { ...next, files: [...next.files, ...smallest.files], tests: [...next.tests, ...smallest.tests], merged: next.merged + smallest.merged + 1 }
  return groups.filter(group => group !== smallest).map(group => (group === next ? merged : group))
}

export const groupFiles = (files: string[]) => {
  let groups = splitDir(files, '', 0)
  while (groups.length > MAX_GROUPS) groups = mergeSmallest(groups)
  return groups
}
