import { expect, test } from 'claude-code/testing'

import { declaredModel, modelToInject } from './declaredModel'

const files = (contents: Record<string, string>) => async (path: string) => {
  if (path in contents) return contents[path]
  throw new Error('absent')
}
const definition = (model?: string) => `---\nname: x\n${model ? `model: ${model}\n` : ''}---\nbody\nmodel: opus\n`

test('the model of the frontmatter is declared', async () => {
  const read = files({ '/p/.claude/agents/mechanic.md': definition('haiku') })
  expect(await declaredModel(read, ['/p', '/h'], 'mechanic')).toBe('haiku')
})

test('the project definition wins over home', async () => {
  const read = files({ '/p/.claude/agents/a.md': definition('haiku'), '/h/.claude/agents/a.md': definition('opus') })
  expect(await declaredModel(read, ['/p', '/h'], 'a')).toBe('haiku')
})

test('home is used when the project has no definition', async () => {
  const read = files({ '/h/.claude/agents/a.md': definition('opus') })
  expect(await declaredModel(read, ['/p', '/h'], 'a')).toBe('opus')
})

test('absent file, missing key, inherit and unsafe types declare nothing', async () => {
  const read = files({ '/p/.claude/agents/none.md': definition(), '/p/.claude/agents/inh.md': definition('inherit') })
  expect(await declaredModel(read, ['/p'], 'absent')).toBeUndefined()
  expect(await declaredModel(read, ['/p'], 'none')).toBeUndefined()
  expect(await declaredModel(read, ['/p'], 'inh')).toBeUndefined()
  expect(await declaredModel(read, ['/p'], 'plugin/a')).toBeUndefined()
  expect(await declaredModel(read, ['/p'], '../a')).toBeUndefined()
  expect(await declaredModel(read, ['/p'], undefined)).toBeUndefined()
})

test('the route model is injected only without caller or declared model', () => {
  expect(modelToInject(undefined, undefined, 'sonnet')).toBe('sonnet')
  expect(modelToInject(undefined, 'haiku', 'sonnet')).toBeUndefined()
  expect(modelToInject('opus', undefined, 'sonnet')).toBeUndefined()
})
