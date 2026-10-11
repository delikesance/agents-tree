import { MAP_FILE } from './mapState'

const GOAL_MAX = 200
export const GOAL_LABEL_PROMPT = `Voici la demande donnée à un agent. Donne en un libellé d'action ce que l'agent va faire ou produire: à l'infinitif, ${GOAL_MAX} caractères maximum, sans pourquoi, sans ponctuation finale. Ne recopie pas la demande, résume-la. Si un contexte est fourni, sers-t'en pour comprendre une demande courte ou allusive (« oui », « continue », « corrige ça »), mais le libellé décrit la demande, pas le contexte. Le libellé est toujours un objectif à accomplir, jamais une question, une demande de précision ni une interaction avec l'utilisateur: si la demande est une question, formule la recherche ou l'explication à produire (« Expliquer… », « Déterminer… »). Réponds uniquement par le libellé.\n\n`
export const withGoalContext = (request: string, context?: string) => (context ? `Contexte (objectif en cours): ${context}\n\nDemande: ${request}` : request)

export const truncateGoal = (line: string) => (line.length > GOAL_MAX ? `${line.slice(0, GOAL_MAX - 1)}…` : line)

export const firstLine = (text: string) => text.split('\n').map(l => l.trim()).find(Boolean) ?? ''

export type Route = { model: string; source: string }

const REPORT_LIMIT_NOTE = 'Final report: 15 lines max, paths and line numbers rather than file contents.'

const REPORT_LIMIT_PRESENT = /\b\d+\s*(lines?|lignes?)\b/i

export const withReportLimit = (prompt: string) => (prompt.includes(REPORT_LIMIT_NOTE) || REPORT_LIMIT_PRESENT.test(prompt) ? prompt : `${prompt}\n\n${REPORT_LIMIT_NOTE}`)
const MAP_POINTER = `Read ${MAP_FILE} first, do not sweep the repo.`

export const withMapPointer = (prompt: string, mapExists: boolean) => (mapExists && !prompt.includes(MAP_FILE) ? `${MAP_POINTER}\n\n${prompt}` : prompt)
export const isQuestion = (line: string) => line.trim().endsWith('?')
