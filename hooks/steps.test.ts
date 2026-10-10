import { expect, test } from 'claude-code/testing'
import { stepsToTodos } from './activity'

test('stepsToTodos marks finished, current and upcoming steps', () => {
  expect(stepsToTodos(['a', 'b', 'c'], 1).map(t => t.status)).toEqual(['completed', 'in_progress', 'pending'])
  expect(stepsToTodos(['a', 'b'], 2).map(t => t.status)).toEqual(['completed', 'completed'])
})
