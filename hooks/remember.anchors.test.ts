import { expect, test } from 'claude-code/testing'
import { parseRemember } from './remember'

test('parseRemember passes anchors through', () => {
  const anchors = [{ path: 'src/auth.ts', symbol: 'refreshToken' }]
  expect(parseRemember({ key: 'k', text: 't', tags: ['auth'], anchors })).toEqual({ entry: { key: 'k', text: 't', tags: ['auth'], anchors } })
})
