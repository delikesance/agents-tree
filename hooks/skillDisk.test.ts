import { expect, test } from 'claude-code/testing'
import { withInstalledSkills } from './skillDisk'
import type { SkillFs, SkillLedger, SkillRecord } from './skillTypes'

const record = (slug: string, status: SkillRecord['status']): SkillRecord => ({
  slug, status, tags: [], sessions: [], signals: [], citedPaths: [], headCommit: '', createdAt: 1, idleSessions: 0, uses: 2, cleanUses: 1,
})

const fsWith = (files: Record<string, string>): SkillFs => ({
  read: async path => files[path],
  write: async () => undefined,
  remove: async () => undefined,
  run: async () => Object.keys(files).map(path => path.split('/').at(-2)).join('\n'),
  ask: async () => '',
})

test('installed skills show up with their description, ledger stats kept', async () => {
  const fs = fsWith({ '/p/.claude/skills/a/SKILL.md': '---\nname: a\ndescription: Use when a\n---', '/p/.claude/skills/b/SKILL.md': '---\ndescription: Use when b\n---' })
  const ledger: SkillLedger = { a: record('a', 'archived') }
  const view = await withInstalledSkills(fs, '/p', ledger)
  expect(view.a).toMatchObject({ status: 'active', uses: 2, summary: 'Use when a' })
  expect(view.b).toMatchObject({ status: 'active', summary: 'Use when b' })
})

test('live ledger skills missing from disk are hidden, candidates kept', async () => {
  const ledger: SkillLedger = { gone: record('gone', 'active'), cand: record('cand', 'candidate') }
  const view = await withInstalledSkills(fsWith({}), '/p', ledger)
  expect(Object.keys(view)).toEqual(['cand'])
})
