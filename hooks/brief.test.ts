import { expect, test } from 'claude-code/testing'

import { BRIEF_HEADER, memoryBrief } from './brief'

test('memory lines become a brief, none gives nothing', () => {
  expect(memoryBrief(['- a', '- b'])).toBe(`${BRIEF_HEADER}\n- a\n- b`)
  expect(memoryBrief([])).toBeUndefined()
})
