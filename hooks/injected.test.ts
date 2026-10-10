import { expect, test } from 'claude-code/testing'

import { CONTEXT_NUDGE, CONTEXT_NUDGE_TOKENS } from './compact'
import { looksMultiStep } from './flow'
import { onlyIfChanged } from './injected'

test('unchanged text is injected once per session', () => {
  expect(onlyIfChanged('s1', 'mode', 'a')).toBe('a')
  expect(onlyIfChanged('s1', 'mode', 'a')).toBeUndefined()
  expect(onlyIfChanged('s1', 'mode', 'b')).toBe('b')
  expect(onlyIfChanged('s2', 'mode', 'b')).toBe('b')
  expect(onlyIfChanged('s1', 'mode', undefined)).toBeUndefined()
})

test('context nudge is imperative and names both commands', () => {
  expect(CONTEXT_NUDGE_TOKENS).toBe(120_000)
  expect(CONTEXT_NUDGE).toContain('/compact')
  expect(CONTEXT_NUDGE).toContain('/clear')
})

test('multi-step prompts are detected, short single asks are not', () => {
  expect(looksMultiStep('corrige la faute dans le titre')).toBe(false)
  expect(looksMultiStep('modifie a.ts et b.ts')).toBe(true)
  expect(looksMultiStep('fais ceci puis cela')).toBe(true)
  expect(looksMultiStep('1. un\n2. deux')).toBe(true)
})
