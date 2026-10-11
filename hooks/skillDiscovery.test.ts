import { expect, test } from 'claude-code/testing'
import { judge, markdownCandidates } from './skillDiscovery'

const README = `# Tests
- run \`bun test hooks/\`
- put a shim in \`node_modules/claude-code/testing.ts\`
- delete \`node_modules\`

## Notes
- one thing
`

test('markdown sections become candidates with their steps, files and commands', () => {
  const [tests] = markdownCandidates(README)
  expect(tests?.slug).toBe('tests')
  expect(tests?.steps.length).toBe(3)
  expect(tests?.files).toEqual(['node_modules/claude-code/testing.ts', 'node_modules'].filter(file => file.includes('.')))
  expect(tests?.commands).toEqual(['bun test hooks/'])
})

test('a grounded multi-step verifiable candidate passes, a short one fails', () => {
  const [tests, notes] = markdownCandidates(README)
  expect(judge(tests!, {}).failed).toEqual([])
  expect(judge(notes!, {}).failed).toEqual(['multi-step', 'grounded', 'verifiable'])
})

test('a candidate already covered by the ledger is not new', () => {
  const [tests] = markdownCandidates(README)
  const ledger = { tests: { slug: 'tests', tags: ['tests'] } } as never
  expect(judge(tests!, ledger).failed).toEqual(['new'])
})
