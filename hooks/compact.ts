export const AUTO_COMPACT_KEY = 'autoCompactEnabled'
export const AUTO_COMPACT_TOKENS = 120_000
export const AUTO_COMPACT_KEEP = 'Keep file paths, symbols, decisions made, and the current goal. Rédige le résumé dans la langue de l\'utilisateur ; garde chemins, symboles, décisions, objectif courant, sortie des tests en échec.'

export const autoCompactState = { armed: true }

const nudgeState = { sent: false }
export const CONTEXT_NUDGE = 'Contexte à 120k. Pour une tâche sans rapport : /rename puis /clear.'

export const contextNudge = (tokens: number) => {
  if (nudgeState.sent || tokens < AUTO_COMPACT_TOKENS) return undefined
  nudgeState.sent = true
  return CONTEXT_NUDGE
}
