import { expect, test } from 'claude-code/testing'
import { buildDigest, MAX_DIGEST_CHARS } from './skillDigest'
import type { SkillDeps } from './skillHost'
import type { SkillSignal } from './skillTypes'

const HOT = 'hooks/a.ts\nhooks/a.test.ts\n\nhooks/a.ts\nhooks/a.test.ts\n\nhooks/a.ts'
const HISTORY = '@@feat x\nhooks/a.ts\nhooks/a.test.ts\n@@feat y\nhooks/a.ts\nhooks/a.test.ts\n@@feat z\nhooks/a.ts\nhooks/a.test.ts'
const FIXES = 'fix: handle empty input\nrevert: drop cache'
const FIX_ARG = '--grep=fix|revert|regress'

const depsWith = (files: Record<string, string>, fixes = FIXES): SkillDeps => {
  const run = async (argv: string[]) => {
    if (argv.includes(FIX_ARG)) return fixes
    if (argv.some(arg => arg.startsWith('--pretty=format:@@'))) return HISTORY
    if (argv.includes('--name-only')) return HOT
    return ''
  }
  return {
    store: { get: async () => undefined, set: async () => {} },
    fs: { read: async path => files[path], write: async () => {}, remove: async () => {}, run, ask: async () => '' },
    now: () => 0,
    sessionId: async () => 's',
  }
}

const CONFIG = { '/repo/.claudework/config.json': '{"test_command":"bun test hooks/"}' }

const signal = (kind: SkillSignal['kind'], note: string): SkillSignal => ({ kind, sessionId: 's', at: 0, files: [], tools: [], note })

test('digest lists commands, hot files with symbols, co-changed groups and fix commits', async () => {
  const digest = await buildDigest(depsWith({ ...CONFIG, '/repo/hooks/a.ts': 'export const run = 1' }), '/repo', [])
  expect(digest).toContain('## Commands\n- Test: `bun test hooks/`')
  expect(digest).toContain('- 3 a.ts(run)')
  expect(digest).toContain('- hooks/a.ts, hooks/a.test.ts')
  expect(digest).toContain('- fix: handle empty input')
})

test('hot files without readable symbols fall back to their path', async () => {
  const digest = await buildDigest(depsWith({}), '/repo', [])
  expect(digest).toContain('- 3 hooks/a.ts')
  expect(digest).not.toContain('## Commands')
})

test('recurring signals keep only corrections and retries, deduplicated', async () => {
  const digest = await buildDigest(
    depsWith({}),
    '/repo',
    [signal('correction', 'use bun'), signal('retry', 'use bun'), signal('task', 'grep everything'), signal('retry', 'run tests twice')],
  )
  expect(digest.split('- use bun').length - 1).toBe(1)
  expect(digest).toContain('- run tests twice')
  expect(digest).not.toContain('grep everything')
})

test('digest stays within the character cap', async () => {
  const longFixes = Array.from({ length: 200 }, (_, n) => `fix ${n}: ${'y'.repeat(60)}`).join('\n')
  const digest = await buildDigest(depsWith({}, longFixes), '/repo', [])
  expect(digest.length).toBeLessThanOrEqual(MAX_DIGEST_CHARS)
})
