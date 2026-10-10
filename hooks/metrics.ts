export type MetricsFs = { read: (path: string) => Promise<string>; write: (path: string, text: string) => Promise<void> }

export type PromptMetrics = { hookLatencyMs: number; injectedChars: number; modelCalls: number }

export const metricsPath = (cwd: string) => `${cwd}/.claudework/metrics/prompts.jsonl`

export async function recordPrompt(fs: MetricsFs, metrics: PromptMetrics, cwd: string): Promise<void> {
  const path = metricsPath(cwd)
  try {
    const previous = await fs.read(path).catch(() => '')
    await fs.write(path, previous + JSON.stringify({ ts: Date.now(), ...metrics }) + '\n')
  } catch {}
}

function parse(line: string): PromptMetrics | undefined {
  try {
    const row = JSON.parse(line)
    return [row?.hookLatencyMs, row?.injectedChars, row?.modelCalls].every(Number.isFinite) ? row : undefined
  } catch {
    return undefined
  }
}

function percentile(sorted: number[], p: number): number {
  if (!sorted.length) return 0
  return sorted[Math.min(sorted.length - 1, Math.ceil(p * sorted.length) - 1)] ?? 0
}

const average = (values: number[]) => (values.length ? values.reduce((a, b) => a + b, 0) / values.length : 0)

export function summarize(lines: string[]) {
  const rows = lines.map(parse).filter((row): row is PromptMetrics => row !== undefined)
  const latencies = rows.map(row => row.hookLatencyMs).sort((a, b) => a - b)
  return {
    count: rows.length,
    p50LatencyMs: percentile(latencies, 0.5),
    p95LatencyMs: percentile(latencies, 0.95),
    avgInjectedChars: average(rows.map(row => row.injectedChars)),
    avgModelCalls: average(rows.map(row => row.modelCalls)),
  }
}
