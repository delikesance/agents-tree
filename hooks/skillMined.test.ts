import { expect, test } from 'claude-code/testing'
import { coChangeClusters, parseHistory } from './skillMined'

const commit = (subject: string, ...files: string[]) => `@@${subject}\n${files.join('\n')}\n`
const LOG = [1, 2, 3].map(n => commit(`c${n}`, 'hooks/a.ts', 'hooks/a.test.ts')).join('\n') + commit('lone', 'hooks/z.ts', 'yarn.lock')

test('files that keep changing together form one cluster', () => {
  expect(coChangeClusters(parseHistory(LOG))).toEqual([{ files: ['hooks/a.ts', 'hooks/a.test.ts'], count: 3 }])
})
