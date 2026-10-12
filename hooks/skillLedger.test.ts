import { expect, test } from 'claude-code/testing'
import { groupSignal, readyToPromote, reject, remove, removeInactive, restore } from './skillLedger'
import type { SkillLedger, SkillSignal } from './skillTypes'

const signal = (sessionId: string, note: string, files: string[] = [], tools: string[] = []): SkillSignal => ({
  kind: 'retry',
  sessionId,
  at: 1,
  files,
  tools,
  note,
})

const NOTE = 'deploy staging database migration'

test('creates a candidate with a kebab slug of at most 40 chars', () => {
  const { ledger, slug } = groupSignal({}, signal('s1', NOTE))
  expect(ledger[slug].status).toBe('candidate')
  expect(slug).toMatch(/^[a-z0-9_]+(-[a-z0-9_]+)*$/)
  expect(slug.length <= 40).toBe(true)
})

test('attaches similar signals to the same record and dedupes sessions', () => {
  const first = groupSignal({}, signal('s1', NOTE))
  const second = groupSignal(first.ledger, signal('s1', NOTE))
  const third = groupSignal(second.ledger, signal('s2', NOTE))
  expect(third.slug).toBe(first.slug)
  expect(Object.keys(third.ledger).length).toBe(1)
  expect(third.ledger[first.slug].sessions).toEqual(['s1', 's2'])
  expect(third.ledger[first.slug].signals.length).toBe(3)
})

test('does not mutate the input ledger', () => {
  const { ledger } = groupSignal({}, signal('s1', NOTE))
  const snapshot = JSON.stringify(ledger)
  groupSignal(ledger, signal('s2', NOTE))
  expect(JSON.stringify(ledger)).toBe(snapshot)
})

test('creates a separate record for unrelated signals', () => {
  const first = groupSignal({}, signal('s1', NOTE))
  const second = groupSignal(first.ledger, signal('s1', 'rename frontend components lint'))
  expect(second.slug).not.toBe(first.slug)
  expect(Object.keys(second.ledger).length).toBe(2)
})

test('caps signals per record', () => {
  let ledger: SkillLedger = {}
  for (let i = 0; i < 30; i++) ledger = groupSignal(ledger, signal('s1', NOTE)).ledger
  expect(Object.values(ledger)[0].signals.length).toBe(20)
})

test('falls back to a default slug when no tags', () => {
  const { slug } = groupSignal({}, signal('s1', ''))
  expect(slug).toBe('skill')
})

test('rejected and archived ideas are not recreated or regrouped', () => {
  const first = groupSignal({}, signal('s1', NOTE))
  const rejected = reject(first.ledger, first.slug)
  const again = groupSignal(rejected, signal('s2', NOTE))
  expect(again.ledger).toBe(rejected)
  expect(again.ledger[first.slug].sessions).toEqual(['s1'])
  const archived: SkillLedger = { [first.slug]: { ...first.ledger[first.slug], status: 'archived' } }
  expect(groupSignal(archived, signal('s2', NOTE)).ledger).toBe(archived)
})

test('reject and restore change status, unknown slug is a no-op', () => {
  const { ledger, slug } = groupSignal({}, signal('s1', NOTE))
  expect(reject(ledger, slug)[slug].status).toBe('rejected')
  expect(restore(reject(ledger, slug), slug)[slug].status).toBe('candidate')
  expect(reject(ledger, 'nope')).toBe(ledger)
})

test('readyToPromote needs candidate, enough sessions and room', () => {
  const base = groupSignal({}, signal('s1', NOTE))
  const record = base.ledger[base.slug]
  const ready = { ...record, sessions: ['a', 'b', 'c'] }
  expect(readyToPromote(ready, 0)).toBe(true)
  expect(readyToPromote({ ...ready, sessions: ['a', 'b'] }, 0)).toBe(false)
  expect(readyToPromote(ready, 20)).toBe(false)
  expect(readyToPromote(ready, 19)).toBe(true)
  expect(readyToPromote({ ...ready, status: 'active' }, 0)).toBe(false)
  expect(readyToPromote({ ...ready, status: 'rejected' }, 0)).toBe(false)
})

test('remove drops only the given record, unknown slug is a no-op', () => {
  const first = groupSignal({}, signal('s1', NOTE))
  const second = groupSignal(first.ledger, signal('s2', 'unrelated refactor of the billing module'))
  const removed = remove(second.ledger, first.slug)
  expect(Object.keys(removed)).toEqual(Object.keys(second.ledger).filter(slug => slug !== first.slug))
  expect(remove(second.ledger, 'nope')).toBe(second.ledger)
})

test('removeInactive keeps candidate and active records only', () => {
  const { ledger, slug } = groupSignal({}, signal('s1', NOTE))
  const record = ledger[slug]
  const statuses: SkillLedger = {
    kept: { ...record, slug: 'kept', status: 'active' },
    candidate: { ...record, slug: 'candidate', status: 'candidate' },
    stale: { ...record, slug: 'stale', status: 'stale' },
    archived: { ...record, slug: 'archived', status: 'archived' },
    rejected: { ...record, slug: 'rejected', status: 'rejected' },
  }
  expect(Object.keys(removeInactive(statuses)).sort()).toEqual(['candidate', 'kept'])
})
