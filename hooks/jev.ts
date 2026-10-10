export const JEV_URL = 'https://api.typesafe.ai/v1/systemone'
export const JEV_MIN_CONFIDENCE = 0.6
export const JEV_TASK_MAX = 400
export const MODEL_CRITERIA = {
  haiku: 'Read-only work: search, list, count, summarize, report. Never modifies files.',
  sonnet: 'Modifies files: write code, fix bugs, make tests pass, rename, refactor, implement features.',
  opus: 'Hard design or architecture decisions with many trade-offs.',
}
export type JevHost = {
  env: { get: (name: string) => Promise<string | undefined> }
  http: { fetch: (url: string, init: { method: string; headers: Record<string, string>; body: string }) => Promise<{ ok: boolean; text: string }> }
}

export type JevChoice = { choice?: string; note: string }
export const jevFailure = (error: unknown): JevChoice => ({ note: `jev: ${error instanceof Error ? error.message : 'erreur'}` })
