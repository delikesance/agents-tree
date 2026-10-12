export type AgentSpawn = (input: { prompt: string; description: string; subagentType: string; model: string }) => Promise<{ agentId?: string; deny?: string }>

const ASK_AGENT = { description: 'skill drafting', subagentType: 'explorer', model: 'haiku' }
const NO_TOOLS_NOTE = 'Answer from the text above only: do not call any tool.'

export function createAgentAsk() {
  const waiting = new Map<string, (answer: string) => void>()
  const spawned = new Set<string>()
  const answers = new Set<string>()
  return {
    ask: async (spawn: AgentSpawn, prompt: string) => {
      const { agentId, deny } = await spawn({ prompt: `${prompt}\n${NO_TOOLS_NOTE}`, ...ASK_AGENT })
      if (!agentId) throw new Error(deny ?? 'spawn refused')
      spawned.add(agentId)
      return new Promise<string>(resolve => waiting.set(agentId, resolve))
    },
    owns: (agentId: string) => spawned.has(agentId),
    isReport: (text: string) => [...spawned].some(id => text.includes(id)) || [...answers].some(answer => text.includes(answer)),
    settle: (agentId: string, answer: string) => {
      if (spawned.has(agentId) && answer.trim()) answers.add(answer.trim())
      waiting.get(agentId)?.(answer)
      waiting.delete(agentId)
    },
  }
}
