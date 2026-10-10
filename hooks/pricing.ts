import type { TurnUsage } from 'claude-code'
import type { AgentUsage } from '../types'

export const PER_MILLION = 1_000_000
export const CACHE_WRITE_FACTOR = 1.25
const FALLBACK_PRICE = { input: 3, output: 15, cacheRead: 0.3 }
type Rate = { input: number; output: number; cacheRead: number }
type Price = Rate & { longPrompt?: Rate }

const LONG_PROMPT_TOKENS = 100_000

// USD per million tokens; matched on a model-id substring
const PRICES: Record<string, Price> = {
  fable: { input: 10, output: 50, cacheRead: 0.25 },
  opus: { input: 4, output: 20, cacheRead: 0.2 },
  sonnet: { input: 2, output: 10, cacheRead: 0.2 },
  haiku: { input: 0.1, output: 0.5, cacheRead: 0.01, longPrompt: { input: 0.5, output: 2.5, cacheRead: 0.05 } },
}
export const priceOf = (model: string, promptTokens = 0): Rate => {
  const price: Price = Object.entries(PRICES).find(([family]) => model.includes(family))?.[1] ?? FALLBACK_PRICE
  return promptTokens > LONG_PROMPT_TOKENS && price.longPrompt ? price.longPrompt : price
}

const costOf = (u: TurnUsage) => {
  const promptTokens = u.input_tokens + u.cache_read_input_tokens + u.cache_creation_input_tokens
  const { input, output, cacheRead } = priceOf(u.model, promptTokens)
  const inputCost = (u.input_tokens + u.cache_creation_input_tokens * CACHE_WRITE_FACTOR) * input + u.cache_read_input_tokens * cacheRead
  return (inputCost + u.output_tokens * output) / PER_MILLION
}

export const addUsage = (previous: AgentUsage | undefined, u: TurnUsage): AgentUsage => ({
  model: u.model,
  input: (previous?.input ?? 0) + u.input_tokens,
  output: (previous?.output ?? 0) + u.output_tokens,
  cacheRead: (previous?.cacheRead ?? 0) + u.cache_read_input_tokens,
  cacheWrite: (previous?.cacheWrite ?? 0) + u.cache_creation_input_tokens,
  cost: (previous?.cost ?? 0) + costOf(u),
  steps: (previous?.steps ?? 0) + 1,
})
const costAt = (u: AgentUsage, model: string) => {
  const { input, output, cacheRead } = priceOf(model)
  return ((u.input + u.cacheWrite * CACHE_WRITE_FACTOR) * input + u.cacheRead * cacheRead + u.output * output) / PER_MILLION
}
export const routedSavings = (usage: Record<string, AgentUsage>, routes: Record<string, string>, baselineModel = '') =>
  Object.keys(routes).reduce((sum, id) => (usage[id] ? sum + Math.max(0, costAt(usage[id], baselineModel) - usage[id].cost) : sum), 0)
