import { CACHE_WRITE_FACTOR, PER_MILLION, priceOf } from './pricing'

export const FLOW_KEY = 'flowEnabled'
export const FLOW_TIMEOUT_MS = 30000
export const FLOW_MAX_AGENTS = 3
export const FLOW_HEADER = 'Flow parallèle lancé par le mod (ne refais pas ces tâches, attends les rapports des agents):'


const TURNS_PER_FEATURE = 15
const SECONDS_PER_TURN = 20
const OUTPUT_TOKENS_PER_TURN = 800
const AGENT_BASE_CONTEXT = 15_000
const AGENT_CONTEXT_GROWTH_PER_TURN = 3_000
const REPORT_TOKENS = 600
const CACHE_TTL_SECONDS = 300
const SPAWN_OVERHEAD_SECONDS = 30
const MIN_GAIN_SECONDS = 120
const COST_TOLERANCE = 1.25
const DEFAULT_MAIN_MODEL = 'sonnet'

export type FeatureKind = 'read' | 'edit' | 'reason'
export type Feature = { title: string; prompt: string; files: string[]; kind: FeatureKind }
export type FlowInput = { features: Feature[]; mainContextTokens: number; mainModel?: string; cacheHitRate: number }
export type FlowDecision = { mode: 'inline' | 'parallel'; reason: string; inlineCost: number; parallelCost: number; secondsSaved: number }

export const FLOW_MAX_TOKENS = 2500
export const CONTEXT_MAX_CHARS = 4000
const CONTEXT_MESSAGES = 4
const MESSAGE_MAX_CHARS = 1200
const RESULT_TAIL_CHARS = 500

export const FLOW_PROMPT = `Tu prépares la demande d'un utilisateur à un agent de code. Le contexte (derniers messages, sorties d'outils, objectif en cours) sert à résoudre les références (« le 1 et le 3 », « fais-le », « oui ») et les demandes vagues (« corrige les bugs » vise les erreurs de lint, de build ou de test visibles dans les sorties d'outils: nomme-les dans le prompt). Réponds uniquement par un JSON {"goal": "...", "features": [...]}.
- "goal": libellé d'action à l'infinitif, 200 caractères maximum, sans ponctuation finale, décrivant ce qui sera fait (jamais une question).
- "features": la demande découpée en fonctionnalités indépendantes (une seule si indissociable, aucune si c'est une question ou un échange). Chacune: "title" (court), "prompt" (consigne complète, précise et autonome pour un agent qui n'a PAS le contexte: ce qu'il faut obtenir, les contraintes et décisions déjà actées, les fichiers et symboles connus, comment vérifier, ce qu'il faut lire avant d'écrire), "files" (fichiers ou dossiers qu'elle modifiera, uniquement ceux qui se déduisent du contexte), "kind" ("read" pour lire, chercher ou explorer, "edit" pour écrire du code, "reason" pour une architecture difficile).
N'invente aucun nom de fichier.\n\n`

type ContextTool = { tool: string; text?: string; isError?: true }
export type ContextMessage = { role: string; text: string; toolUses: ContextTool[] }

const tail = (text: string, max: number) => (text.length > max ? `…${text.slice(-max)}` : text)
const describeTool = ({ tool, text, isError }: ContextTool) => `[${tool}${isError ? ' ERREUR' : ''}] ${tail((text ?? '').trim(), RESULT_TAIL_CHARS)}`
const describeMessage = ({ role, text, toolUses }: ContextMessage) =>
  [text.trim() && `${role}: ${tail(text.trim(), MESSAGE_MAX_CHARS)}`, ...toolUses.map(describeTool)].filter(Boolean).join('\n')

export const recentContext = (messages: ContextMessage[]) => messages.slice(-CONTEXT_MESSAGES).map(describeMessage).filter(Boolean).join('\n\n')

export type Plan = { goal?: string; features: Feature[] }

const isKind = (kind: unknown): kind is FeatureKind => kind === 'read' || kind === 'edit' || kind === 'reason'

const toFeature = (raw: Record<string, unknown>): Feature | undefined =>
  typeof raw.title === 'string' && typeof raw.prompt === 'string' && isKind(raw.kind) && Array.isArray(raw.files)
    ? { title: raw.title, prompt: raw.prompt, kind: raw.kind, files: raw.files.filter((file): file is string => typeof file === 'string') }
    : undefined

export const parsePlan = (text: string): Plan => {
  try {
    const { goal, features } = JSON.parse(text.slice(text.indexOf('{'), text.lastIndexOf('}') + 1))
    return {
      goal: typeof goal === 'string' && goal.trim() ? goal.trim() : undefined,
      features: Array.isArray(features) ? features.map(toFeature).filter((feature): feature is Feature => !!feature) : [],
    }
  } catch {
    return { features: [] }
  }
}

export const planInput = (request: string, context: string) => (context ? `Contexte:\n${context.slice(-CONTEXT_MAX_CHARS)}\n\nDemande: ${request}` : `Demande: ${request}`)

const normalized = (path: string) => path.replace(/^\.\//, '').replace(/\/+$/, '')
const overlaps = (a: string, b: string) => a === b || a.startsWith(`${b}/`) || b.startsWith(`${a}/`)

export const hasDisjointFiles = (features: Feature[]) =>
  features.every((feature, i) => (feature.kind === 'read' || feature.files.length > 0) && features.slice(i + 1).every(other => !feature.files.some(a => other.files.some(b => overlaps(normalized(a), normalized(b))))))

export const EXPLORE_TIMEOUT_MS = 20000
const FINDINGS_HEADER = 'Repères trouvés par un explorateur (lis seulement ces fichiers, sinon demande un explorer):'
const EXPLORER = { subagentType: 'explorer', model: 'haiku' }

export const needsExploration = (feature: Feature) => feature.kind !== 'read'

export const explorerPrompt = (feature: Feature) =>
  `Lecture seule. Pour préparer cette tâche, donne au plus 15 lignes: fichiers (avec lignes), symboles à modifier, tests concernés, fonctions réutilisables. Aucun extrait de fichier.${feature.files.length ? ` Périmètre: ${feature.files.join(', ')}.` : ''}\n\nTâche: ${feature.prompt}`

export const withFindings = (feature: Feature, findings?: string): Feature =>
  findings?.trim() ? { ...feature, prompt: `${feature.prompt}\n\n${FINDINGS_HEADER}\n${findings.trim()}` } : feature

export const agentFor = (feature: Feature) => (feature.kind === 'read' ? EXPLORER : { subagentType: 'worker', model: 'sonnet' })

const turnCost = (model: string, contextTokens: number, readShare: number) => {
  const { input, output, cacheRead } = priceOf(model, contextTokens)
  return (contextTokens * (readShare * cacheRead + (1 - readShare) * input * CACHE_WRITE_FACTOR) + OUTPUT_TOKENS_PER_TURN * output) / PER_MILLION
}

const inlineCost = ({ features, mainContextTokens, mainModel = DEFAULT_MAIN_MODEL, cacheHitRate }: FlowInput) =>
  features.length * TURNS_PER_FEATURE * turnCost(mainModel, mainContextTokens, cacheHitRate)

const delegatedAgentCost = (agent: { model: string }) => {
  const averageContext = AGENT_BASE_CONTEXT + (AGENT_CONTEXT_GROWTH_PER_TURN * TURNS_PER_FEATURE) / 2
  const { input } = priceOf(agent.model, AGENT_BASE_CONTEXT)
  return (AGENT_BASE_CONTEXT * input * CACHE_WRITE_FACTOR) / PER_MILLION + TURNS_PER_FEATURE * turnCost(agent.model, averageContext, 1)
}

const delegatedFeatureCost = (feature: Feature) => delegatedAgentCost(agentFor(feature)) + (needsExploration(feature) ? delegatedAgentCost(EXPLORER) : 0)

const parallelCost = (input: FlowInput) => {
  const { mainModel = DEFAULT_MAIN_MODEL, mainContextTokens } = input
  const waitSeconds = TURNS_PER_FEATURE * SECONDS_PER_TURN
  const cacheRewrite = waitSeconds > CACHE_TTL_SECONDS ? turnCost(mainModel, mainContextTokens, 0) : 0
  const reports = (input.features.length * REPORT_TOKENS * priceOf(mainModel).cacheRead * TURNS_PER_FEATURE) / PER_MILLION
  return input.features.reduce((sum, feature) => sum + delegatedFeatureCost(feature), cacheRewrite + reports)
}

const inline = (reason: string): FlowDecision => ({ mode: 'inline', reason, inlineCost: 0, parallelCost: 0, secondsSaved: 0 })

export const decideFlow = (input: FlowInput): FlowDecision => {
  const { features } = input
  if (features.length < 2) return inline('moins de 2 fonctionnalités')
  if (!hasDisjointFiles(features)) return inline('fichiers qui se chevauchent ou inconnus')
  const sequential = inlineCost(input)
  const parallel = parallelCost(input)
  const secondsSaved = (features.length - 1) * TURNS_PER_FEATURE * SECONDS_PER_TURN - SPAWN_OVERHEAD_SECONDS
  const decision = { inlineCost: sequential, parallelCost: parallel, secondsSaved }
  if (secondsSaved < MIN_GAIN_SECONDS) return { ...decision, mode: 'inline', reason: 'gain de temps trop faible' }
  if (parallel > sequential * COST_TOLERANCE) return { ...decision, mode: 'inline', reason: 'agents plus chers que le contexte en cache' }
  return { ...decision, mode: 'parallel', reason: 'fonctionnalités indépendantes, coût comparable ou moindre' }
}

export const ownershipNote = (feature: Feature) =>
  feature.files.length ? `\n\nNe modifie que: ${feature.files.join(', ')}. Ne fais aucun commit.` : '\n\nNe fais aucun commit.'

export const directive = (spawned: Feature[], rest: Feature[], decision: FlowDecision) => {
  const lines = spawned.map(feature => `- ${feature.title} (${agentFor(feature).subagentType})`)
  const remaining = rest.length ? `\nÀ traiter toi-même: ${rest.map(feature => feature.title).join(', ')}.` : ''
  const cost = `~$${decision.parallelCost.toFixed(2)} en agents contre ~$${decision.inlineCost.toFixed(2)} en séquence, ~${Math.round(decision.secondsSaved / 60)} min gagnées`
  return `${FLOW_HEADER}\n${lines.join('\n')}${remaining}\nSuis l'avancement avec TodoWrite, puis relis et vérifie leurs rapports. (${cost})`
}

const MULTI_STEP_MIN_CHARS = 150
const MULTI_STEP_FILES = 2
const MULTI_STEP_LIST_ITEMS = 2
const FILE_REFERENCE = /[\w./-]+\.\w{1,5}\b/g
const LIST_ITEM = /^\s*(?:\d+[.)]|[-*])\s/gm
const SEQUENCE_WORDS = /\b(puis|ensuite|après ça|et aussi|plusieurs fichiers|refactor\w*|migr\w+|then|also|all files|step by step)\b/i

export const looksMultiStep = (text: string) =>
  text.length >= MULTI_STEP_MIN_CHARS ||
  (text.match(FILE_REFERENCE)?.length ?? 0) >= MULTI_STEP_FILES ||
  (text.match(LIST_ITEM)?.length ?? 0) >= MULTI_STEP_LIST_ITEMS ||
  SEQUENCE_WORDS.test(text)
