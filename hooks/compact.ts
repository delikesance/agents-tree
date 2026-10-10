export const AUTO_COMPACT_KEY = 'autoCompactEnabled'
export const AUTO_COMPACT_TOKENS = 120_000
export const AUTO_COMPACT_KEEP = 'Keep file paths, symbols, decisions made, and the current goal. Rédige le résumé dans la langue de l\'utilisateur ; garde chemins, symboles, décisions, objectif courant, sortie des tests en échec.'

export const autoCompactState = { armed: true }

const nudgeState = { sent: false }
export const CONTEXT_NUDGE_TOKENS = 120_000
export const CONTEXT_NUDGE = 'STOP: le contexte atteint ~120k tokens. Arrête-toi maintenant et demande à l\'utilisateur de lancer /compact (même tâche) ou /clear (nouvelle tâche, après /rename) avant de continuer.'

export const contextNudge = (tokens: number) => {
  if (nudgeState.sent || tokens < CONTEXT_NUDGE_TOKENS) return undefined
  nudgeState.sent = true
  return CONTEXT_NUDGE
}
