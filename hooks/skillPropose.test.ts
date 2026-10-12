import { expect, test } from 'claude-code/testing'
import { proposeCandidates } from './skillPropose'
import type { SkillFs } from './skillTypes'

const fsReturning = (ask: SkillFs['ask']): SkillFs => ({ read: async () => undefined, write: async () => {}, remove: async () => {}, run: async () => '', ask })

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
