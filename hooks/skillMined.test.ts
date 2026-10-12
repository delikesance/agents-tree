import { expect, test } from 'claude-code/testing'
import { minedCandidates, parseHistory } from './skillMined'
import { judge } from './skillDiscovery'

const commit = (subject: string, ...files: string[]) => `@@${subject}\n${files.join('\n')}\n`
const LOG = [1, 2, 3].map(n => commit(`c${n}`, 'hooks/a.ts', 'hooks/a.test.ts')).join('\n') + commit('lone', 'hooks/z.ts', 'yarn.lock')

test('files that keep changing together become a verifiable candidate', () => {
  const [candidate] = minedCandidates(parseHistory(LOG), ['bun test hooks/'])
  expect(candidate?.slug).toBe('modifier-a')
  expect(candidate?.files).toEqual(['hooks/a.ts', 'hooks/a.test.ts'])
  expect(judge(candidate!, {}).failed).toEqual([])
})

test('without a known test command the candidate is not verifiable', () => {
  const [candidate] = minedCandidates(parseHistory(LOG), [])
  expect(judge(candidate!, {}).failed).toEqual(['multi-step', 'verifiable'])
})
