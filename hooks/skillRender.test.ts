import { expect, test } from 'claude-code/testing'
import { renderSkill, validateSkill } from './skillRender'

const draft = { slug: 'fix-tests', description: 'Use when tests fail after a refactor', steps: ['Run tests', 'Fix'], citedPaths: ['src/a.ts'] }
const withDescription = (description: string) => renderSkill({ ...draft, description })

test('renderSkill: frontmatter, steps and files', () => {
  const text = renderSkill(draft)
  expect(text.startsWith('---\nname: fix-tests\ndescription: Use when tests fail after a refactor\n---\n')).toBe(true)
  expect(text).toContain('1. Run tests')
  expect(text).toContain('- src/a.ts')
  expect(validateSkill(text)).toEqual([])
})

test('validateSkill: description rules', () => {
  expect(validateSkill(withDescription(''))).toContain('description is empty')
  expect(validateSkill(withDescription('Fix tests'))).toContain("description must start with 'Use when'")
  expect(validateSkill(withDescription('Use when ' + 'x'.repeat(200)))[0]).toContain('exceeds')
  expect(validateSkill(withDescription('Use when a\nsecond line'))).toContain('description spans multiple lines')
})

test('validateSkill: missing frontmatter and long body', () => {
  expect(validateSkill('hello')).toEqual(['missing frontmatter'])
  const steps = Array.from({ length: 45 }, (_, i) => `step ${i}`)
  expect(validateSkill(renderSkill({ ...draft, steps }))).toContain('body exceeds 40 lines')
})

test('validateSkill: a .claude/ path is not a forbidden mention', () => {
  expect(validateSkill(renderSkill({ ...draft, steps: ['Edit `.claude/project-map.md`'] }))).not.toContain('contains a forbidden mention')
})

test('validateSkill: forbidden content', () => {
  expect(validateSkill(renderSkill({ ...draft, steps: ['Ask Claude'] }))).toContain('contains a forbidden mention')
  expect(validateSkill(renderSkill({ ...draft, steps: ['Use the AI'] }))).toContain('contains a forbidden mention')
  expect(validateSkill(renderSkill({ ...draft, steps: ['export KEY=sk-abcdefghijklmnop1234'] }))).toContain('contains a secret-looking string')
  expect(validateSkill(renderSkill({ ...draft, steps: ['Edit /home/bob/x'] }))).toContain('contains an absolute home path')
})
