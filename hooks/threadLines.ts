import { describeCall } from './activity'

export type ThreadLine = { kind: 'user' | 'assistant' | 'tool' | 'error'; text: string; tool?: string }
type Row = { role: string; text: string; toolUses?: readonly { tool: string; input: Record<string, unknown>; isError?: boolean }[] }

const toolLine = ({ tool, input, isError }: NonNullable<Row['toolUses']>[number]): ThreadLine => ({ kind: isError ? 'error' : 'tool', tool, text: describeCall({ tool, ...input }) })

export const threadLines = (rows: readonly Row[], max: number): ThreadLine[] =>
  rows
    .flatMap(({ role, text, toolUses = [] }): ThreadLine[] => [...(text.trim() ? [{ kind: role === 'user' ? 'user' : 'assistant', text: text.trim() } as ThreadLine] : []), ...toolUses.map(toolLine)])
    .slice(-max)

export const relayTarget = (agentId: string, text: string) => (text.trim() ? { to: { agentId }, text: text.trim() } : undefined)

export const firstPrompt = (rows: readonly Row[]) => rows.find(({ role, text }) => role === 'user' && text.trim())?.text.trim()
