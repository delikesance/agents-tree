export const BRIEF_KEY = 'briefEnabled'
export const BRIEF_HEADER = 'Mémoire pertinente:'
export const BRIEF_TIMEOUT_MS = 3000

export const memoryBrief = (lines: string[]) => (lines.length ? `${BRIEF_HEADER}\n${lines.join('\n')}` : undefined)

export const withTimeout = async <T,>(work: Promise<T>, ms: number) => {
  let timer: ReturnType<typeof setTimeout> | undefined
  const timeout = new Promise<undefined>(resolve => {
    timer = setTimeout(resolve, ms)
  })
  try {
    return await Promise.race([work, timeout])
  } finally {
    clearTimeout(timer)
  }
}
