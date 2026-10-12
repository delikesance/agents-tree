import { expect, test } from 'claude-code/testing'
import { judge, markdownCandidates } from './skillDiscovery'
import { verdictLines } from './skillDiscover'

const README = `# Tests
- run \`bun test hooks/\`
- put a shim in \`node_modules/claude-code/testing.ts\`
- delete \`node_modules\`

## Notes
- one thing
`

test('markdown sections become candidates with their steps and files', () => {
  const [tests] = markdownCandidates(README)
  expect(tests?.slug).toBe('tests')
  expect(tests?.steps.length).toBe(3)
  expect(tests?.files).toEqual(['node_modules/claude-code/testing.ts', 'node_modules'].filter(file => file.includes('.')))
})

test('a grounded multi-step candidate passes, a short one fails', () => {
  const [tests, notes] = markdownCandidates(README)
  expect(judge(tests!, {}).failed).toEqual([])
  expect(judge(notes!, {}).failed).toEqual(['multi-step', 'grounded'])
})

test('a candidate already covered by the ledger is not new', () => {
  const [tests] = markdownCandidates(README)
  const ledger = { tests: { slug: 'tests', tags: ['tests'] } } as never
  expect(judge(tests!, ledger).failed).toEqual(['new'])
})

const CLAUDE = `# Conventions
- run \`bun test hooks/\`
- put a shim in \`node_modules/claude-code/testing.ts\`
- delete \`node_modules\`
`

const FRESH = `# Widgets
- sketch the widget layout in \`hooks/widget.tsx\`
- measure the render budget with \`bun test hooks/widget.test.ts\`
- ship the widget through \`hooks/register.tsx\`
`

test('a candidate whose steps all appear in the permanent context is rejected with its coverage', () => {
  const [tests] = markdownCandidates(README)
  const verdict = judge(tests!, {}, [CLAUDE])
  expect(verdict.failed).toEqual(['not-in-context'])
  expect(verdict.covered).toBe(3)
})

test('a candidate with steps absent from the permanent context is accepted', () => {
  const [widgets] = markdownCandidates(FRESH)
  expect(judge(widgets!, {}, [CLAUDE]).failed).toEqual([])
})

test('the verdict line states that a candidate is already in the permanent context', () => {
  const [tests] = markdownCandidates(README)
  expect(verdictLines([judge(tests!, {}, [CLAUDE])])).toBe('✘ tests — déjà dans le contexte permanent (3/3 étapes)')
})
