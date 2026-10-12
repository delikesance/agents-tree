import { expect, test } from 'claude-code/testing'
import { removeSkill, skillPath, writeSkill } from './skillWriter'
import type { SkillFs, SkillRecord } from './skillTypes'

const memoryFs = (files: Record<string, string> = {}): SkillFs & { files: Record<string, string> } => ({
  files,
  read: async path => files[path],
  write: async (path, text) => { files[path] = text },
  remove: async path => { delete files[path] },
  run: async () => 'abc123\n',
})

const record = (overrides: Partial<SkillRecord> = {}): SkillRecord => ({
  slug: 'fix-tests', status: 'candidate', tags: [], sessions: [], signals: [], citedPaths: ['a.ts', 'gone.ts'],
  headCommit: '', createdAt: 0, idleSessions: 0, uses: 0, cleanUses: 0, ...overrides,
})
const synthesized = { description: 'Use when tests fail', steps: ['Run tests in a.ts'] }

test('writeSkill: writes SKILL.md and keeps only existing cited paths', async () => {
  const fs = memoryFs({ '/p/a.ts': 'x' })
  const result = await writeSkill(fs, '/p', record(), synthesized)
  expect(result).toEqual({ ok: true, citedPaths: ['a.ts'], headCommit: 'abc123' })
  expect(fs.files[skillPath('/p', 'fix-tests')]).toContain('name: fix-tests')
})

test('writeSkill: refuses invalid content', async () => {
  const fs = memoryFs()
  const result = await writeSkill(fs, '/p', record(), { description: 'Fix tests', steps: ['a.ts'] })
  expect(result.ok).toBe(false)
  expect(Object.keys(fs.files)).toEqual([])
})

test('writeSkill: rejects unsafe slugs', async () => {
  const result = await writeSkill(memoryFs(), '/p', record({ slug: '../x' }), synthesized)
  expect(result).toEqual({ ok: false, errors: ['invalid slug'] })
})

test('removeSkill: deletes the skill file and rejects unsafe slugs', async () => {
  const fs = memoryFs({ [skillPath('/p', 'a')]: 'x' })
  expect(await removeSkill(fs, '/p', 'a/b')).toBe(false)
  expect(await removeSkill(fs, '/p', '..')).toBe(false)
  expect(await removeSkill(fs, '/p', 'a')).toBe(true)
  expect(fs.files).toEqual({})
})

test('a discovered skill lists only the files its steps mention, yet keeps tracking all cited files', async () => {
  const written: Record<string, string> = {}
  const fs = { read: async () => 'x', write: async (path: string, text: string) => void (written[path] = text), remove: async () => undefined, run: async () => 'abc\n' }
  const record = { slug: 'tests', citedPaths: ['a.ts', 'b.ts'], signals: [{ kind: 'discovery' }] } as never
  const result = await writeSkill(fs, '/p', record, { description: 'Use when testing', steps: ['edit a.ts'] })
  expect(result).toMatchObject({ ok: true, citedPaths: ['a.ts', 'b.ts'] })
  const text = Object.values(written)[0] ?? ''
  expect(text).toContain('- a.ts')
  expect(text).not.toContain('- b.ts')
})

test('writeSkill: refuses steps that cite no project file', async () => {
  const fs = memoryFs({ '/p/a.ts': 'x' })
  const result = await writeSkill(fs, '/p', record(), { description: 'Use when tests fail', steps: ['Run tests'] })
  expect(result).toEqual({ ok: false, errors: ['steps cite no project file'] })
})
