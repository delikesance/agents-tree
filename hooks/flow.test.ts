import { expect, test } from 'claude-code/testing'

import { decideFlow, explorerPrompt, hasDisjointFiles, needsExploration, parsePlan, planInput, recentContext, withFindings } from './flow'
import type { Feature } from './flow'

const feature = (title: string, files: string[], kind: Feature['kind'] = 'edit'): Feature => ({ title, prompt: title, files, kind })
const base = { mainModel: 'claude-sonnet-5-5', cacheHitRate: 0.9 }

test('coupled features stay inline', () => {
  const features = [feature('a', ['src/x.go']), feature('b', ['src/x.go'])]
  expect(decideFlow({ ...base, features, mainContextTokens: 150_000 }).mode).toBe('inline')
})

test('disjoint features with a large context go parallel', () => {
  const features = [feature('a', ['src/a']), feature('b', ['src/b']), feature('c', ['docs'], 'read')]
  expect(decideFlow({ ...base, features, mainContextTokens: 150_000 }).mode).toBe('parallel')
})

test('a single feature stays inline', () => {
  expect(decideFlow({ ...base, features: [feature('a', ['x'])], mainContextTokens: 150_000 }).mode).toBe('inline')
})

test('unknown files on an edit are not disjoint', () => {
  expect(hasDisjointFiles([feature('a', []), feature('b', ['y'])])).toBe(false)
})

test('parses the plan reply and drops malformed features', () => {
  const plan = parsePlan('ok {"goal":" Corriger le tri ","features":[{"title":"t","prompt":"p","files":["a"],"kind":"edit"},{"title":1}]}')
  expect(plan.goal).toBe('Corriger le tri')
  expect(plan.features).toHaveLength(1)
  expect(parsePlan('nope')).toEqual({ features: [] })
})

test('plan input keeps the tail of the context', () => {
  expect(planInput('fais le 1', '')).toBe('Demande: fais le 1')
  expect(planInput('fais le 1', 'x'.repeat(5000) + 'FIN')).toContain('FIN')
})

test('recent context keeps tool errors and drops old messages', () => {
  const messages = [
    { role: 'user', text: 'old', toolUses: [] },
    ...Array.from({ length: 3 }, () => ({ role: 'assistant', text: 'step', toolUses: [] })),
    { role: 'assistant', text: '', toolUses: [{ tool: 'Bash', text: `${'x'.repeat(900)}error: could not compile`, isError: true as const }] },
  ]
  const context = recentContext(messages)
  expect(context).not.toContain('old')
  expect(context).toContain('[Bash ERREUR]')
  expect(context.endsWith('error: could not compile')).toBe(true)
})

test('only non-read features get an exploration, and findings land in the prompt', () => {
  expect(needsExploration(feature('a', ['x'], 'read'))).toBe(false)
  expect(needsExploration(feature('a', ['x']))).toBe(true)
  expect(withFindings(feature('a', ['x']), '  src/x.go:10  ').prompt).toContain('src/x.go:10')
  expect(withFindings(feature('a', ['x']), undefined).prompt).toBe('a')
  expect(explorerPrompt(feature('a', ['src/a']))).toContain('src/a')
})
