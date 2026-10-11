export type WorkflowAgent = { id: string; runId: string; agentIndex: number; type: string; startedAt: number; endedAt?: number }

const agents = new Map<string, WorkflowAgent>()

export const recordWorkflowAgent = (agent: WorkflowAgent) => void agents.set(agent.id, agent)

export const finishWorkflowAgent = (id: string, now: number) => {
  const agent = agents.get(id)
  if (agent && agent.endedAt === undefined) agent.endedAt = now
}

export const workflowAgents = (): WorkflowAgent[] => [...agents.values()]

export const workflowNodeId = (runId: string) => `workflow:${runId}`

export const workflowLabel = (runId: string) => `workflow · ${runId.replace(/^wf_/, '').slice(0, 8)}`

export const groupByRun = (members: readonly WorkflowAgent[]): Map<string, WorkflowAgent[]> => {
  const runs = new Map<string, WorkflowAgent[]>()
  members.forEach(member => runs.set(member.runId, [...(runs.get(member.runId) ?? []), member]))
  runs.forEach(group => group.sort((a, b) => a.agentIndex - b.agentIndex))
  return runs
}

export const forgetWorkflowAgents = () => agents.clear()
