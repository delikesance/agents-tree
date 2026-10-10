export const AUTO_COMPACT_KEY = 'autoCompactEnabled'
export const AUTO_COMPACT_TOKENS = 120_000
export const AUTO_COMPACT_KEEP = 'Keep file paths, symbols, decisions made, and the current goal. Rédige le résumé dans la langue de l\'utilisateur ; garde chemins, symboles, décisions, objectif courant, sortie des tests en échec.'

export const autoCompactState = { armed: true }

const DEFAULT_SESSION = 'default'
const nudgedSessions = new Set<string>()
export const CONTEXT_NUDGE_TOKENS = 120_000
export const CONTEXT_NUDGE = 'STOP: le contexte atteint ~120k tokens. Arrête-toi maintenant et demande à l\'utilisateur de lancer /compact (même tâche) ou /clear (nouvelle tâche, après /rename) avant de continuer.'

export const contextNudge = (tokens: number, sessionId = DEFAULT_SESSION) => {
  if (nudgedSessions.has(sessionId) || tokens < CONTEXT_NUDGE_TOKENS) return undefined
  nudgedSessions.add(sessionId)
  return CONTEXT_NUDGE
}

export const resetNudge = (sessionId = DEFAULT_SESSION) => {
  nudgedSessions.delete(sessionId)
}
