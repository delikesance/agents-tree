import { expect, test } from 'claude-code/testing'

import { finishWorkflowAgent, forgetWorkflowAgents, groupByRun, recordWorkflowAgent, workflowAgents, workflowLabel } from './workflowRuns'

const member = (id: string, runId: string, agentIndex: number) => ({ id, runId, agentIndex, type: 'worker', label: id, startedAt: 1 })

test('groupByRun orders each run by agent index', () => {
  const runs = groupByRun([member('b', 'wf_1', 2), member('c', 'wf_2', 1), member('a', 'wf_1', 1)])
  expect(runs.get('wf_1')?.map(({ id }) => id)).toEqual(['a', 'b'])
  expect(runs.get('wf_2')?.map(({ id }) => id)).toEqual(['c'])
})

test('finishing a workflow agent stamps its end once', () => {
  forgetWorkflowAgents()
  recordWorkflowAgent(member('a', 'wf_1', 1))
  finishWorkflowAgent('a', 5)
  finishWorkflowAgent('a', 9)
  finishWorkflowAgent('unknown', 9)
  expect(workflowAgents().map(({ endedAt }) => endedAt)).toEqual([5])
})

test('the run label drops the wf_ prefix and shortens the id', () => {
  expect(workflowLabel('wf_abcdefghijkl')).toBe('workflow · abcdefgh')
})
