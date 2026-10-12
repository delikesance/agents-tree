import type { SkillEvent, SkillSignal, SkillSignalKind } from './skillTypes'

type ToolEvent = Extract<SkillEvent, { type: 'tool' }>

const MAX_SESSIONS = 50
const MAX_TURN_EVENTS = 200
const MAX_SIGNAL_FILES = 12
const NOTE_CHARS = 160
const MIN_RETRY_FAILURES = 2
const MIN_EXPLORATION_READS = 8
const READ_TOOLS = new Set(['Read', 'Grep', 'Glob'])
const EDIT_TOOLS = new Set(['Edit', 'Write'])
const CORRECTION_CUES =
  /(?<![\p{L}])(non|pas [çc]a|plutôt|instead|actually|wrong|revert|undo|ne fais pas)(?![\p{L}])/iu

type SessionState = {
  turn: ToolEvent[]
  failedTool?: string
  failures: number
  explored: boolean
  turnEnded: boolean
}

const freshState = (): SessionState => ({ turn: [], failures: 0, explored: false, turnEnded: false })

const unique = <T>(items: T[]) => [...new Set(items)]

const signal = (kind: SkillSignalKind, events: ToolEvent[], at: number, note: string, sessionId: string): SkillSignal => ({
  kind,
  sessionId,
  at,
  files: unique(events.flatMap((event) => event.files)).slice(0, MAX_SIGNAL_FILES),
  tools: unique(events.map((event) => event.tool)),
  note,
})

const isSuccessfulEdit = (event: ToolEvent) => EDIT_TOOLS.has(event.tool) && !event.failed

function retrySignal(state: SessionState, event: ToolEvent): SkillSignal[] {
  if (event.failed) {
    state.failures = state.failedTool === event.tool ? state.failures + 1 : 1
    state.failedTool = event.tool
    return []
  }
  const recovered = state.failedTool === event.tool && state.failures >= MIN_RETRY_FAILURES
  const failures = state.failures
  state.failedTool = undefined
  state.failures = 0
  if (!recovered) return []
  return [signal('retry', [event], event.at, `${event.tool} failed ${failures} times then succeeded`, event.sessionId)]
}

function explorationSignal(state: SessionState, event: ToolEvent): SkillSignal[] {
  if (state.explored || !isSuccessfulEdit(event)) return []
  state.explored = true
  const reads = state.turn.filter((call) => READ_TOOLS.has(call.tool))
  if (reads.length < MIN_EXPLORATION_READS) return []
  return [signal('exploration', reads, event.at, `${reads.length} reads before first edit`, event.sessionId)]
}

function correctionSignal(state: SessionState, event: Extract<SkillEvent, { type: 'prompt' }>): SkillSignal[] {
  const afterTurn = state.turnEnded
  state.turnEnded = false
  if (!afterTurn || !CORRECTION_CUES.test(event.text)) return []
  return [{ kind: 'correction', sessionId: event.sessionId, at: event.at, files: [], tools: [], note: event.text.slice(0, NOTE_CHARS) }]
}

function onTool(state: SessionState, event: ToolEvent): SkillSignal[] {
  state.turnEnded = false
  if (state.turn.length >= MAX_TURN_EVENTS) state.turn.shift()
  state.turn.push(event)
  return [...retrySignal(state, event), ...explorationSignal(state, event)]
}

function onTurnEnd(state: SessionState): SkillSignal[] {
  state.turn = []
  state.explored = false
  state.turnEnded = true
  return []
}

export function createSignalDetector() {
  const sessions = new Map<string, SessionState>()

  const stateFor = (sessionId: string) => {
    const known = sessions.get(sessionId)
    if (known) return known
    if (sessions.size >= MAX_SESSIONS) sessions.delete(sessions.keys().next().value as string)
    const created = freshState()
    sessions.set(sessionId, created)
    return created
  }

  return {
    push(event: SkillEvent): SkillSignal[] {
      const state = stateFor(event.sessionId)
      if (event.type === 'tool') return onTool(state, event)
      if (event.type === 'turn-end') return onTurnEnd(state)
      if (event.type === 'prompt') return correctionSignal(state, event)
      return []
    },
    endSession(sessionId: string) {
      sessions.delete(sessionId)
    },
  }
}
