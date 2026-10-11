export type WorkflowAgent = { id: string; runId: string; agentIndex: number; type: string; label: string; startedAt: number; endedAt?: number }

const agents = new Map<string, WorkflowAgent>()
const LAUNCH_GRACE_MS = 60_000
let launches: number[] = []

export const recordWorkflowLaunch = (now: number) => void launches.push(now)

export const workflowLaunches = (now: number): number[] => (launches = launches.filter(at => now - at < LAUNCH_GRACE_MS))

export const recordWorkflowAgent = (agent: WorkflowAgent) => {
  launches.shift()
  agents.set(agent.id, agent)
}

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

export const forgetWorkflowAgents = () => {
  agents.clear()
  launches = []
}
