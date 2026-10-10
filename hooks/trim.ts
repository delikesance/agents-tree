const BASH_MAX_LINES = 400
const BASH_HEAD_LINES = 150
const BASH_TAIL_LINES = 100
const CHARS_PER_TOKEN = 4
const trimOutput = (output: string) => {
  const lines = output.split('\n')
  if (lines.length <= BASH_MAX_LINES) return undefined
  const omitted = lines.length - BASH_HEAD_LINES - BASH_TAIL_LINES
  const kept = [...lines.slice(0, BASH_HEAD_LINES), `… ${omitted} lignes omises par agent-graph: relire une plage (offset/limit ou grep) si nécessaire …`, ...lines.slice(-BASH_TAIL_LINES)]
  return { text: kept.join('\n'), savedTokens: Math.round(lines.slice(BASH_HEAD_LINES, -BASH_TAIL_LINES).join('\n').length / CHARS_PER_TOKEN) }
}

export const trimResultContent = (content: unknown, onSaved: (tokens: number) => void): unknown => {
  if (typeof content === 'string') {
    const trimmed = trimOutput(content)
    if (!trimmed) return content
    onSaved(trimmed.savedTokens)
    return trimmed.text
  }
  if (!Array.isArray(content)) return content
  return content.map(block => (block?.type === 'text' ? { ...block, text: trimResultContent(block.text, onSaved) } : block))
}
