import { expect, test } from 'claude-code/testing'
import { proposeCandidates } from './skillPropose'
import type { SkillFs } from './skillTypes'

const fsReturning = (run: SkillFs['run']): SkillFs => ({ read: async () => undefined, write: async () => {}, remove: async () => {}, run })

const skill = {
  title: 'Run the hook tests after editing skills',
  steps: ['Edit `hooks/skillMined.ts`', 'Update `hooks/skillMined.test.ts`', 'Run `bun test hooks/skill`'],
}

test('proposeCandidates: maps model skills to grounded candidates', async () => {
  const candidates = await proposeCandidates(fsReturning(async () => JSON.stringify({ skills: [skill] })), 'digest', [])
  expect(candidates).toEqual([
    {
      slug: 'run-the-hook-tests-after-editing-skills',
      title: skill.title,
      steps: skill.steps,
      files: ['hooks/skillMined.ts', 'hooks/skillMined.test.ts'],
      commands: ['bun test hooks/skill'],
    },
  ])
})

test('proposeCandidates: drops skills with too few steps', async () => {
  const short = { title: 'Short', steps: ['Run `bun test`'] }
  const candidates = await proposeCandidates(fsReturning(async () => JSON.stringify({ skills: [short, skill] })), 'digest', [])
  expect(candidates.map(candidate => candidate.title)).toEqual([skill.title])
})

test('proposeCandidates: keeps at most five skills', async () => {
  const many = Array.from({ length: 7 }, (_, index) => ({ ...skill, title: `Skill ${index}` }))
  const candidates = await proposeCandidates(fsReturning(async () => JSON.stringify({ skills: many })), 'digest', [])
  expect(candidates.map(candidate => candidate.title)).toEqual(['Skill 0', 'Skill 1', 'Skill 2', 'Skill 3', 'Skill 4'])
})

test('proposeCandidates: returns [] for malformed JSON', async () => {
  expect(await proposeCandidates(fsReturning(async () => 'not json {'), 'digest', [])).toEqual([])
  expect(await proposeCandidates(fsReturning(async () => 'no braces at all'), 'digest', [])).toEqual([])
})

test('proposeCandidates: returns [] when the model call rejects', async () => {
  expect(await proposeCandidates(fsReturning(async () => { throw new Error('boom') }), 'digest', [])).toEqual([])
})

test('a proposal without a command gets the known test command as its last step', async () => {
  const output = JSON.stringify({ skills: [{ title: 'Edit a', steps: ['Read `hooks/a.ts`', 'Edit `hooks/a.ts`', 'Check `hooks/b.ts`'] }] })
  const [candidate] = await proposeCandidates({ run: async () => output } as never, 'digest', [], 'bun test hooks/')
  expect(candidate?.commands).toEqual(['bun test hooks/'])
})
