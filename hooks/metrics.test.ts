import { expect, test } from 'claude-code/testing'
import { metricsPath, recordPrompt, summarize } from './metrics'

const row = (hookLatencyMs: number) => JSON.stringify({ hookLatencyMs, injectedChars: 100, modelCalls: 1 })

const memoryFs = (files: Record<string, string> = {}) => ({
  files,
  read: async (path: string) => (path in files ? files[path] : Promise.reject(new Error('missing'))),
  write: async (path: string, text: string) => void (files[path] = text),
})

test('summarize: computes percentiles and averages', () => {
  const s = summarize([row(10), row(20), row(30), row(40)])
  expect(s).toEqual({ count: 4, p50LatencyMs: 20, p95LatencyMs: 40, avgInjectedChars: 100, avgModelCalls: 1 })
})

test('summarize: skips unparsable and empty lines', () => {
  expect(summarize(['{bad', '', row(5)]).count).toBe(1)
})

test('summarize: skips rows with a missing or non-numeric field', () => {
  const rows = [JSON.stringify({ hookLatencyMs: 1, injectedChars: 2 }), JSON.stringify({ hookLatencyMs: 1, injectedChars: '2', modelCalls: 0 }), row(5)]
  expect(summarize(rows).count).toBe(1)
})

test('summarize: empty input yields zeros', () => {
  expect(summarize([]).p95LatencyMs).toBe(0)
})

test('recordPrompt: appends one JSON line', async () => {
  const fs = memoryFs()
  await recordPrompt(fs, { hookLatencyMs: 3, injectedChars: 0, modelCalls: 0 }, '/p')
  await recordPrompt(fs, { hookLatencyMs: 4, injectedChars: 0, modelCalls: 0 }, '/p')
  expect(summarize(fs.files[metricsPath('/p')].split('\n')).count).toBe(2)
})

test('recordPrompt: never throws on an unwritable path', async () => {
  const fs = { read: async () => '', write: () => Promise.reject(new Error('denied')) }
  await expect(recordPrompt(fs, { hookLatencyMs: 1, injectedChars: 0, modelCalls: 0 }, '/dev/null')).resolves.toBeUndefined()
})
