import { expect, test } from 'claude-code/testing'

import { firstPrompt, relayTarget, threadLines } from './threadLines'

test('thread keeps text and tool calls, newest last, capped', () => {
  const rows = [
    { role: 'user', text: 'fais X', toolUses: [] },
    { role: 'assistant', text: 'ok', toolUses: [{ tool: 'Read', input: { file_path: 'a.ts' } }, { tool: 'Bash', input: { command: 'ls' }, isError: true }] },
  ]
  expect(threadLines(rows, 10).map(l => l.kind)).toEqual(['user', 'assistant', 'tool', 'error'])
  expect(threadLines(rows, 2).map(l => l.kind)).toEqual(['tool', 'error'])
})

test('empty rows give no lines', () => {
  expect(threadLines([{ role: 'assistant', text: '  ', toolUses: [] }], 5)).toEqual([])
})

test('a blank reply is not sent, a real one targets the agent', () => {
  expect(relayTarget('a1', '   ')).toBeUndefined()
  expect(relayTarget('a1', ' stop ')).toEqual({ to: { agentId: 'a1' }, text: 'stop' })
})

test('firstPrompt returns the first non-empty user message', () => {
  expect(firstPrompt([{ role: 'assistant', text: 'hi' }, { role: 'user', text: ' do it ' }, { role: 'user', text: 'later' }])).toBe('do it')
})
