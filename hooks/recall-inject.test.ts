import { expect, test } from 'claude-code/testing'

import { hydrateMemories, rememberOutcome } from './knowledge'
import { relevantMemories } from './recall-inject'

test('returns matching entries as one-line bullets, best first, capped by limit', () => {
  hydrateMemories([])
  rememberOutcome('auth-refresh', 'Token refresh\nlives in src/auth.ts', ['auth', 'refresh'])
  rememberOutcome('billing', 'Invoices in src/billing.ts', ['billing'])
  const lines = relevantMemories('how does auth refresh work?')
  expect(lines).toEqual(['- auth-refresh: Token refresh lives in src/auth.ts'])
  expect(relevantMemories('auth billing refresh', 1)).toHaveLength(1)
})

test('returns nothing below the threshold and truncates long text', () => {
  hydrateMemories([])
  rememberOutcome('long', 'x'.repeat(500), ['database'])
  expect(relevantMemories('unrelated question')).toEqual([])
  expect(relevantMemories('database')[0]).toHaveLength('- long: '.length + 160)
})

test('rememberOutcome masks secrets', () => {
  hydrateMemories([])
  rememberOutcome('creds', 'password=hunter2 deploy notes', ['deploy'])
  expect(relevantMemories('deploy')[0]).not.toContain('hunter2')
})
