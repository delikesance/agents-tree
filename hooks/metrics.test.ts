import { describe, expect, test } from 'bun:test'
import { mkdtempSync, readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { metricsPath, recordPrompt, summarize } from './metrics'

const row = (hookLatencyMs: number) => JSON.stringify({ hookLatencyMs, injectedChars: 100, modelCalls: 1 })

describe('summarize', () => {
  test('computes percentiles and averages', () => {
    const s = summarize([row(10), row(20), row(30), row(40)])
    expect(s).toEqual({ count: 4, p50LatencyMs: 20, p95LatencyMs: 40, avgInjectedChars: 100, avgModelCalls: 1 })
  })

  test('skips unparsable and empty lines', () => {
    expect(summarize(['{bad', '', row(5)]).count).toBe(1)
  })

  test('empty input yields zeros', () => {
    expect(summarize([]).p95LatencyMs).toBe(0)
  })
})

describe('recordPrompt', () => {
  test('appends one JSON line', () => {
    const cwd = mkdtempSync(join(tmpdir(), 'metrics-'))
    recordPrompt({ hookLatencyMs: 3, injectedChars: 0, modelCalls: 0 }, cwd)
    expect(summarize(readFileSync(metricsPath(cwd), 'utf8').split('\n')).count).toBe(1)
  })

  test('never throws on an unwritable path', () => {
    expect(() => recordPrompt({ hookLatencyMs: 1, injectedChars: 0, modelCalls: 0 }, '/dev/null')).not.toThrow()
  })
})
