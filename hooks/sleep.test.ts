import { expect, test } from 'claude-code/testing'
import { spriteLines } from './sprites'

test('sleeping sprite shows a z mark and differs from the idle one', () => {
  const asleep = spriteLines('sonnet', undefined, 900, true)
  expect(asleep[2]?.at(-2)?.char).toBe('z')
  expect(spriteLines('sonnet', undefined, 2700, true)[0]?.at(-1)?.char).toBe('Z')
  expect(asleep).not.toEqual(spriteLines('sonnet', undefined, 2700))
})
