import type { Group } from './mapGroups'

const MAX_LISTED_PARTS = 4
const WHOLE_DIRECTORY = '*'

const groupSize = ({ files, tests }: Group) => files.length + tests.length

const mergedName = (parent: string, parts: string[]) => {
  const sorted = [...parts].sort()
  return `${parent}/{${parts.length <= MAX_LISTED_PARTS ? parts.join(',') : `${sorted[0]}..${sorted.at(-1)}`}}`
}

const mergeGroups = (target: Group, absorbed: Group): Group => {
  const parts = [...target.parts, ...absorbed.parts]
  return { ...target, name: mergedName(target.parent, parts), parts, files: [...target.files, ...absorbed.files], tests: [...target.tests, ...absorbed.tests] }
}

const collapsePrefixed = (groups: Group[], parent: string): Group[] => {
  const members = groups.filter(group => group.prefixed && group.parent === parent)
  const whole: Group = {
    name: `${parent}/${WHOLE_DIRECTORY}`,
    files: members.flatMap(group => group.files),
    tests: members.flatMap(group => group.tests),
    parent,
    parts: [WHOLE_DIRECTORY],
  }
  return groups.flatMap(group => (group === members[0] ? whole : members.includes(group) ? [] : group))
}

export const mergeSmallest = (groups: Group[]) => {
  const bySize = [...groups].sort((a, b) => groupSize(a) - groupSize(b))
  for (const smallest of bySize) {
    const sibling = bySize.find(group => group !== smallest && group.parent === smallest.parent && !group.prefixed === !smallest.prefixed)
    if (!sibling) continue
    if (smallest.prefixed) return collapsePrefixed(groups, smallest.parent)
    return groups.filter(group => group !== smallest).map(group => (group === sibling ? mergeGroups(sibling, smallest) : group))
  }
  return groups
}
