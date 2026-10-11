import { expect, test } from 'claude-code/testing'

import { UNLEARNABLE_PREFIXES, isLearnable } from './learnable'

test('boilerplate reports are rejected, with or without leading whitespace', () => {
  for (const prefix of UNLEARNABLE_PREFIXES) {
    expect(isLearnable(prefix)).toBe(false)
    expect(isLearnable(`  \n${prefix} and more`)).toBe(false)
  }
})

test('a normal report is accepted', () => {
  expect(isLearnable('hooks/flow.ts:12 caps agents at 3')).toBe(true)
})
