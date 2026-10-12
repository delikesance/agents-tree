import { expect, test } from 'claude-code/testing'
import { closeSession } from './skillMaintenance'
import type { SkillDeps } from './skillHost'
import type { SkillLedger, SkillRecord } from './skillTypes'

const record = (slug: string, citedPaths: string[]): SkillRecord => ({
  slug, status: 'active', tags: [], sessions: [], signals: [], citedPaths, headCommit: '', createdAt: 0, idleSessions: 0, uses: 0, cleanUses: 0,
})

test('closeSession: archives active skills that cite no file and removes their file', async () => {
  const removed: string[] = []
  const deps = { cwd: '/p', fs: { remove: async (path: string) => { removed.push(path) } } } as unknown as SkillDeps
  const ledger: SkillLedger = { vague: record('vague', []), specific: record('specific', ['src/a.ts']) }
  const closed = await closeSession(deps, ledger, 's1')
  expect(closed.vague.status).toBe('archived')
  expect(closed.specific.status).toBe('active')
  expect(removed).toEqual(['/p/.claude/skills/vague/SKILL.md'])
})
