import { expect, test } from 'claude-code/testing'

import { cardKind } from './cardStatus'
import type { Node } from './tree'

const node = (overrides: Partial<Node> = {}): Node => ({ id: 'a', label: 'a', status: 'completed', reveal: 1, children: [], ...overrides })
const todo = (status: string) => ({ content: 'x', status })

test('gone and failed statuses win', () => {
  expect(cardKind(node({ status: 'gone' }))).toBe('gone')
  expect(cardKind(node({ status: 'failed' }))).toBe('failed')
  expect(cardKind(node({ status: 'killed' }))).toBe('failed')
})

test('a live unfinished agent is running', () => {
  expect(cardKind(node({ status: 'running', todos: [todo('pending')] }))).toBe('running')
  expect(cardKind(node({ status: 'waiting' }))).toBe('running')
})

test('a completed agent is done only when its work is over', () => {
  expect(cardKind(node())).toBe('done')
  expect(cardKind(node({ todos: [todo('completed')] }))).toBe('done')
  expect(cardKind(node({ todos: [todo('completed'), todo('pending')] }))).toBe('paused')
  expect(cardKind(node({ progress: { done: 1, total: 3 } }))).toBe('paused')
  expect(cardKind(node({ progress: { done: 3, total: 3 } }))).toBe('done')
})

test('an idle agent is paused unless every todo is completed', () => {
  expect(cardKind(node({ status: 'idle' }))).toBe('paused')
  expect(cardKind(node({ status: 'idle', todos: [todo('completed')] }))).toBe('done')
})

test('a live descendant keeps the card paused', () => {
  const grandchild = node({ id: 'c', status: 'running' })
  const child = node({ id: 'b', children: [grandchild] })
  expect(cardKind(node({ children: [child] }))).toBe('paused')
})

test('a finished main turn with todos left is paused', () => {
  expect(cardKind(node({ status: 'running', finished: true, todos: [todo('pending')] }))).toBe('paused')
  expect(cardKind(node({ status: 'running', finished: true }))).toBe('done')
})
