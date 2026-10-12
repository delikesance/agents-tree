import { expect, test } from 'claude-code/testing'
import { closeSession, flagStale } from './skillMaintenance'
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

test('flagStale: only deleted cited files flag a skill, plain edits do not', async () => {
  const commands: string[][] = []
  const deleted = ['src/gone.ts']
  const deps = { cwd: '/p', fs: { run: async (command: string[]) => { commands.push(command); return command.includes('--diff-filter=D') ? deleted.join('\n') : 'src/a.ts' } } } as unknown as SkillDeps
  const ledger: SkillLedger = { kept: { ...record('kept', ['src/a.ts']), headCommit: 'h' }, gone: { ...record('gone', ['src/gone.ts']), headCommit: 'h' } }
  const flagged = await flagStale(deps, ledger)
  expect(flagged.kept.status).toBe('active')
  expect(flagged.gone.status).toBe('stale')
  expect(commands[0]).toContain('--diff-filter=D')
})
