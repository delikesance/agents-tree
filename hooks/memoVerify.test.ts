import { expect, test } from 'claude-code/testing'
import { pruneStale, verifyAnchors } from './memoVerify'
import type { AnchorDeps } from './memoVerify'
import type { Entry } from './knowledge'

const files: Record<string, string> = { 'src/auth.ts': 'export function refreshToken() {}' }

const deps: AnchorDeps = {
  exists: async path => path in files,
  read: async path => files[path],
}

const entry = (key: string, anchors?: Entry['anchors']): Entry => ({ key, tags: [], text: key, at: 0, anchors })

test('entries without anchors are ok', async () => {
  expect(await verifyAnchors(entry('plain'), deps)).toBe('ok')
})

test('anchor with existing path and symbol is ok', async () => {
  expect(await verifyAnchors(entry('a', [{ path: 'src/auth.ts', symbol: 'refreshToken' }]), deps)).toBe('ok')
})

test('anchor with a missing path is stale', async () => {
  expect(await verifyAnchors(entry('a', [{ path: 'src/gone.ts' }]), deps)).toBe('stale')
})

test('anchor whose symbol vanished from the file is stale', async () => {
  expect(await verifyAnchors(entry('a', [{ path: 'src/auth.ts', symbol: 'logout' }]), deps)).toBe('stale')
})

test('any broken anchor makes the entry stale', async () => {
  const anchors = [{ path: 'src/auth.ts', symbol: 'refreshToken' }, { path: 'src/gone.ts' }]
  expect(await verifyAnchors(entry('a', anchors), deps)).toBe('stale')
})

test('pruneStale drops stale entries, learned ones included', async () => {
  const learned: Entry = { ...entry('learned', [{ path: 'src/gone.ts' }]), learned: true }
  const kept = entry('kept', [{ path: 'src/auth.ts', symbol: 'refreshToken' }])
  const plain = entry('plain')
  expect(await pruneStale([learned, kept, plain], deps)).toEqual([kept, plain])
})
