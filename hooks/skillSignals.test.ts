import { expect, test } from 'claude-code/testing'
import { createSignalDetector } from './skillSignals'
import type { SkillEvent } from './skillTypes'

const tool = (name: string, files: string[] = [], failed = false, sessionId = 's1'): SkillEvent => ({
  type: 'tool', sessionId, at: 1, tool: name, files, failed,
})
const prompt = (text: string): SkillEvent => ({ type: 'prompt', sessionId: 's1', at: 2, text })
const turnEnd = (sessionId = 's1'): SkillEvent => ({ type: 'turn-end', sessionId, at: 3, turns: 1, corrected: false })

const feed = (events: SkillEvent[], detector = createSignalDetector()) => events.flatMap((event) => detector.push(event))
const kinds = (events: SkillEvent[]) => feed(events).map((signal) => signal.kind)

test('a plain read-edit chain emits nothing, even at turn end', () => {
  expect(kinds([tool('Grep', ['src/a.ts']), tool('Read', ['src/a.ts']), tool('Edit', ['src/a.ts']), turnEnd()])).toEqual([])
})

test('correction: cue after a turn end carries truncated text', () => {
  const text = `non ${'x'.repeat(300)}`
  const signals = feed([turnEnd(), prompt(text)])
  expect(signals.map((signal) => signal.kind)).toEqual(['correction'])
  expect(signals[0].note).toHaveLength(160)
  expect(kinds([turnEnd(), prompt('Fais plutôt ceci')])).toEqual(['correction'])
  expect(kinds([turnEnd(), prompt('please revert that')])).toEqual(['correction'])
})

test('correction: no cue, no prior turn, or cue inside a word emit nothing', () => {
  expect(kinds([turnEnd(), prompt('continue please')])).toEqual([])
  expect(kinds([prompt('non, instead do it')])).toEqual([])
  expect(kinds([turnEnd(), prompt('the nonce is fine')])).toEqual([])
})

test('retry: two failures then success of the same tool', () => {
  const signals = feed([tool('Bash', [], true), tool('Bash', [], true), tool('Bash', ['a.sh'])])
  expect(signals.map((signal) => signal.kind)).toEqual(['retry'])
  expect(signals[0].tools).toEqual(['Bash'])
})

test('retry: single failure, mixed tools, or no recovery emit nothing', () => {
  expect(kinds([tool('Bash', [], true), tool('Bash')])).toEqual([])
  expect(kinds([tool('Bash', [], true), tool('Edit', [], true), tool('Bash')])).toEqual([])
  expect(kinds([tool('Bash', [], true), tool('Bash', [], true)])).toEqual([])
})

test('exploration: eight reads before first edit', () => {
  const reads = Array.from({ length: 8 }, (_, index) => tool('Read', [`f${index}.ts`]))
  expect(kinds([...reads, tool('Edit', ['f0.ts'])])).toContain('exploration')
})

test('exploration: seven reads or reads after the edit emit nothing', () => {
  const reads = (count: number) => Array.from({ length: count }, () => tool('Grep'))
  expect(kinds([...reads(7), tool('Write')])).not.toContain('exploration')
  expect(kinds([tool('Edit'), ...reads(9), tool('Edit')])).not.toContain('exploration')
})

test('signal files are deduped and capped at 12', () => {
  const many = Array.from({ length: 20 }, (_, index) => tool('Read', [`d/f${index}.ts`, 'd/f0.ts']))
  const signals = feed([...many, tool('Edit', ['d/f0.ts'])])
  expect(signals[0].files).toHaveLength(12)
  expect(new Set(signals[0].files).size).toBe(12)
})

test('state is bounded: turn events are capped and old sessions are evicted', () => {
  const detector = createSignalDetector()
  feed(Array.from({ length: 500 }, () => tool('Read', ['a.ts'])), detector)
  feed([tool('Edit', ['a.ts'])], detector)
  expect(detector.push(turnEnd())).toEqual([])

  feed([turnEnd('old')], detector)
  for (let index = 0; index < 60; index += 1) detector.push(tool('Read', [], false, `other${index}`))
  expect(detector.push({ type: 'prompt', sessionId: 'old', at: 1, text: 'non' })).toEqual([])
})

test('endSession drops state', () => {
  const detector = createSignalDetector()
  detector.push(turnEnd())
  detector.endSession('s1')
  expect(detector.push(prompt('non'))).toEqual([])
})
