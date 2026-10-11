import { expect, test } from 'claude-code/testing'
import { endSessionTick, markStale, recordUse, revalidate, underperforming } from './skillLifecycle'
import type { SkillLedger, SkillRecord } from './skillTypes'

const record = (slug: string, change: Partial<SkillRecord> = {}): SkillRecord => ({
  slug,
  status: 'active',
  tags: [slug],
  sessions: [],
  signals: [],
  citedPaths: [],
  headCommit: 'old',
  createdAt: 0,
  idleSessions: 0,
  uses: 0,
  cleanUses: 0,
  ...change,
})

const ledgerOf = (...records: SkillRecord[]): SkillLedger => Object.fromEntries(records.map(r => [r.slug, r]))

test('recordUse counts uses, clean uses and resets idle', () => {
  const ledger = ledgerOf(record('a', { idleSessions: 4 }))
  const used = recordUse(recordUse(ledger, 'a', 10, true), 'a', 20, false)
  expect(used.a).toMatchObject({ uses: 2, cleanUses: 1, idleSessions: 0, lastUsedAt: 20 })
  expect(ledger.a.uses).toBe(0)
})

test('recordUse ignores unknown slug and tracks session once', () => {
  const ledger = ledgerOf(record('a'))
  expect(recordUse(ledger, 'zzz', 1, true)).toBe(ledger)
  const used = recordUse(recordUse(ledger, 'a', 1, true, 's1'), 'a', 2, true, 's1')
  expect(used.a.sessions).toEqual(['s1'])
})

test('endSessionTick idles active skills not used in the session', () => {
  const ledger = ledgerOf(record('a'), record('b', { sessions: ['s1'] }), record('c', { status: 'candidate' }))
  const next = endSessionTick(ledger, 's1')
  expect(next.a.idleSessions).toBe(1)
  expect(next.b.idleSessions).toBe(0)
  expect(next.c.idleSessions).toBe(0)
})

test('endSessionTick archives at the idle threshold', () => {
  const ledger = ledgerOf(record('a', { idleSessions: 9 }), record('b', { idleSessions: 8 }), record('c', { status: 'stale', idleSessions: 20 }))
  const next = endSessionTick(ledger, 's1')
  expect(next.a.status).toBe('archived')
  expect(next.b.status).toBe('active')
  expect(next.c.status).toBe('stale')
})

test('markStale matches exact paths and path prefixes on active records only', () => {
  const ledger = ledgerOf(
    record('a', { citedPaths: ['hooks/x.ts'] }),
    record('b', { citedPaths: ['hooks/dir/y.ts'] }),
    record('c', { citedPaths: ['other/z.ts'] }),
    record('d', { citedPaths: ['hooks/x.ts'], status: 'candidate' }),
    record('e', { citedPaths: ['hooks/xtra.ts'] }),
  )
  const next = markStale(ledger, ['hooks/x.ts', 'hooks/dir/'])
  expect(next.a.status).toBe('stale')
  expect(next.b.status).toBe('stale')
  expect(next.c.status).toBe('active')
  expect(next.d.status).toBe('candidate')
  expect(next.e.status).toBe('active')
})

test('markStale with no changes leaves records alone', () => {
  const ledger = ledgerOf(record('a', { citedPaths: ['x'] }))
  expect(markStale(ledger, []).a.status).toBe('active')
})

test('revalidate returns a stale skill to active with the new head', () => {
  const ledger = ledgerOf(record('a', { status: 'stale' }), record('b', { status: 'archived' }))
  expect(revalidate(ledger, 'a', 'new').a).toMatchObject({ status: 'active', headCommit: 'new' })
  expect(revalidate(ledger, 'b', 'new').b.status).toBe('archived')
  expect(revalidate(ledger, 'zzz', 'new')).toBe(ledger)
})

test('underperforming needs 5 uses and a clean ratio under 0.5', () => {
  expect(underperforming(record('a', { uses: 5, cleanUses: 2 }))).toBe(true)
  expect(underperforming(record('a', { uses: 5, cleanUses: 3 }))).toBe(false)
  expect(underperforming(record('a', { uses: 4, cleanUses: 0 }))).toBe(false)
  expect(underperforming(record('a', { uses: 10, cleanUses: 5 }))).toBe(false)
})
