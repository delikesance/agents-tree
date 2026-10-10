const GOAL_MAX = 200
export const GOAL_LABEL_PROMPT = `Voici la demande donnée à un agent. Donne en un libellé d'action ce que l'agent va faire ou produire: à l'infinitif, ${GOAL_MAX} caractères maximum, sans pourquoi, sans ponctuation finale. Ne recopie pas la demande, résume-la. Si un contexte est fourni, sers-t'en pour comprendre une demande courte ou allusive (« oui », « continue », « corrige ça »), mais le libellé décrit la demande, pas le contexte. Le libellé est toujours un objectif à accomplir, jamais une question, une demande de précision ni une interaction avec l'utilisateur: si la demande est une question, formule la recherche ou l'explication à produire (« Expliquer… », « Déterminer… »). Réponds uniquement par le libellé.\n\n`
export const withGoalContext = (request: string, context?: string) => (context ? `Contexte (objectif en cours): ${context}\n\nDemande: ${request}` : request)

export const truncateGoal = (line: string) => (line.length > GOAL_MAX ? `${line.slice(0, GOAL_MAX - 1)}…` : line)

export const firstLine = (text: string) => text.split('\n').map(l => l.trim()).find(Boolean) ?? ''
const MODEL_TIERS = ['haiku', 'sonnet', 'opus']
export const ROUTER_PROMPT = `Tu routes une tâche déléguée à un sous-agent pour minimiser le coût sans perdre en qualité.
Choisis le modèle le moins cher qui suffit: "haiku" pour lire, chercher, lister, compter, résumer; "sonnet" pour écrire ou modifier du code, déboguer, analyser; "opus" seulement pour un raisonnement ou une architecture vraiment difficiles.
Réécris aussi le prompt: précis, autonome, avec l'objectif, les chemins ou éléments concernés, les contraintes et une limite de taille de la réponse. Garde tous les faits du prompt d'origine, n'en invente aucun.
Réponds uniquement par un JSON {"model": "...", "prompt": "..."}.\n\nPrompt d'origine:\n`

export type Route = { model: string; prompt: string; source: string }

export const parseRoute = (text: string): Route | undefined => {
  try {
    const { model, prompt } = JSON.parse(text.slice(text.indexOf('{'), text.lastIndexOf('}') + 1))
    return MODEL_TIERS.includes(model) && typeof prompt === 'string' && prompt.trim() ? { model, prompt, source: 'haiku' } : undefined
  } catch {
    return undefined
  }
}
export const ROUTER_MIN_PROMPT = 200
const REPORT_LIMIT_NOTE = 'Final report: 15 lines max, paths and line numbers rather than file contents.'

const REPORT_LIMIT_PRESENT = /\b\d+\s*(lines?|lignes?)\b/i

export const withReportLimit = (prompt: string) => (prompt.includes(REPORT_LIMIT_NOTE) || REPORT_LIMIT_PRESENT.test(prompt) ? prompt : `${prompt}\n\n${REPORT_LIMIT_NOTE}`)
export const isQuestion = (line: string) => line.trim().endsWith('?')
