export const BRIEF_KEY = 'briefEnabled'
export const BRIEF_HEADER = 'Mémoire pertinente:'
export const BRIEF_TIMEOUT_MS = 3000

export const memoryBrief = (lines: string[]) => (lines.length ? `${BRIEF_HEADER}\n${lines.join('\n')}` : undefined)

export const withTimeout = <T,>(work: Promise<T>, ms: number) =>
  Promise.race([work, new Promise<undefined>(resolve => setTimeout(resolve, ms))])
