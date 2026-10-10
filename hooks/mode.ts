export const MODE_KEY = 'modeEnabled'
export const MODE_HEADER = 'Mode conseillé pour cette demande:'
export const MODE_CRITERIA = {
  normal: 'Small, clear, single-step request: a question, a quick fix, a lookup, a one-file edit.',
  plan: 'Non-trivial change touching several files or with design choices: explore and write a plan before editing.',
  goal: 'Large objective with a verifiable end state that needs many steps and a progress bar.',
  loop: 'Recurring or polling work: repeat a check or task until a condition is met.',
}
const MODE_ADVICE: Record<string, string> = {
  plan: "commence par entrer en plan mode (EnterPlanMode): explore, propose un plan, attends la validation avant d'éditer.",
  goal: "découpe en étapes, annonce le total avec l'outil progress, puis avance étape par étape jusqu'à l'état final vérifiable.",
  loop: "traite la demande comme une boucle: vérifie, agis, revérifie, et arrête-toi quand la condition est remplie ou après deux échecs identiques.",
}
export const modeAdvice = (mode: string | undefined) => (mode && MODE_ADVICE[mode] ? `${MODE_HEADER} ${MODE_ADVICE[mode]}` : undefined)
