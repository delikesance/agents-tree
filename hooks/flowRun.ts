import { withTimeout } from './brief'
import { EXPLORE_TIMEOUT_MS, FLOW_MAX_AGENTS, agentFor, decideFlow, directive, explorerPrompt, needsExploration, ownershipNote, withFindings } from './flow'
import type { Feature } from './flow'

type SpawnInput = { prompt: string; description: string; subagentType: string; model: string }
type ExploreInput = { tool: 'Agent'; subagent_type: string; model: 'haiku'; description: string; prompt: string }

export type FlowActions = {
  explore: (input: ExploreInput) => Promise<{ text?: string }>
  spawn: (input: SpawnInput) => Promise<{ deny?: string }>
}

export type MainState = { tokens: number; model?: string; cacheHitRate: number }

const explore = async ({ explore: call }: FlowActions, feature: Feature) => {
  const result = await withTimeout(call({ tool: 'Agent', subagent_type: 'explorer', model: 'haiku', description: `Explorer: ${feature.title}`, prompt: explorerPrompt(feature) }), EXPLORE_TIMEOUT_MS).catch(() => undefined)
  return result?.text
}

const prepare = async (actions: FlowActions, feature: Feature) => (needsExploration(feature) ? withFindings(feature, await explore(actions, feature)) : feature)

const spawnFeature = async ({ spawn }: FlowActions, feature: Feature) => {
  const { deny } = await spawn({ prompt: feature.prompt + ownershipNote(feature), description: feature.title, ...agentFor(feature) })
  return !deny
}

export const runFlow = async (actions: FlowActions, features: Feature[], main: MainState) => {
  const decision = decideFlow({ features, mainContextTokens: main.tokens, mainModel: main.model, cacheHitRate: main.cacheHitRate })
  if (decision.mode === 'inline') return undefined
  const [batch, rest] = [features.slice(0, FLOW_MAX_AGENTS), features.slice(FLOW_MAX_AGENTS)]
  const started = (await Promise.all(batch.map(async feature => ((await spawnFeature(actions, await prepare(actions, feature))) ? feature : undefined)))).filter((feature): feature is Feature => !!feature)
  return started.length ? directive(started, [...rest, ...batch.filter(feature => !started.includes(feature))], decision) : undefined
}
