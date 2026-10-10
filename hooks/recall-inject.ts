import { currentMemories, rank } from './knowledge'

const MIN_POINTS = 3
const LINE_TEXT_CHARS = 160

const oneLine = (text: string) => text.replace(/\s+/g, ' ').trim()

export function relevantMemories(prompt: string, limit = 3): string[] {
  return rank(currentMemories(), prompt, limit, MIN_POINTS).map(({ key, text }) => `- ${key}: ${oneLine(text).slice(0, LINE_TEXT_CHARS)}`)
}
