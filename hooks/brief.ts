export const BRIEF_KEY = 'briefEnabled'
export const BRIEF_HEADER = "Pistes d'orientation (à vérifier vite, sinon cherche normalement):"
export const BRIEF_MAX_HITS = 2
export const BRIEF_TIMEOUT_MS = 3000
const TITLE_MAX_CHARS = 60
export const briefTitle = (key: string) => key.split(':')[0].trim().slice(0, TITLE_MAX_CHARS)

export const withTimeout = <T,>(work: Promise<T>, ms: number) =>
  Promise.race([work, new Promise<undefined>(resolve => setTimeout(resolve, ms))])
