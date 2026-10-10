import { atom, read, update } from 'claude-code'
import type { Register } from 'claude-code'

import type { AgentUsage, GoalProgress } from '../types'

import { PROGRESS_SECTION, PROGRESS_TOOL } from './progress'
import type { Todo } from './activity'
import { TASK_STATUS_DONE, countTask, describeCall, stepsToTodos, todoProgress, toTodos } from './activity'
import { BREAKER_DENY, BREAKER_FAILURES, RETRY_OR_STOP, evictOldest, recordFailure, recordSuccess, resetBreaker, startCall, stopped } from './breaker'
import { stashFlow, takeFlow } from './backgroundFlow'
import { BRIEF_KEY, BRIEF_TIMEOUT_MS, memoryBrief, withTimeout } from './brief'
import { AUTO_COMPACT_KEEP, AUTO_COMPACT_KEY, AUTO_COMPACT_TOKENS, autoCompactState, contextNudge, resetNudge } from './compact'
import { FLOW_KEY, FLOW_MAX_TOKENS, FLOW_PROMPT, FLOW_TIMEOUT_MS, looksMultiStep, parsePlan, planInput, recentContext } from './flow'
import type { Plan } from './flow'
import { runFlow } from './flowRun'
import type { FlowActions } from './flowRun'
import { recordPrompt } from './metrics'
import { relevantMemories } from './recall-inject'
import { parseRemember } from './remember'
import { onlyIfChanged } from './injected'
import { makeBanner } from './banner'
import { makeContext } from './contextSection'
import { cacheHitRate, clampedPercent, compactionLimit, contextSegments, contextSummary, percent, mostUrgent, rateBars, usageLines } from './format'
import { ICONS } from './icons'
import { MODE_CRITERIA, MODE_KEY, modeAdvice } from './mode'
import { JEV_MIN_CONFIDENCE, JEV_TASK_MAX, JEV_URL, MODEL_CRITERIA, jevFailure } from './jev'
import type { JevChoice, JevHost } from './jev'
import { KNOWLEDGE_SECTION, LEARNED_KEY_CHARS, MAX_LEARNED_TAGS, RECALL_TOOL, REMEMBER_TOOL, SECRET_MASK, SECRET_PATTERN, STORE_KEY, clip, hydrateMemories, search, tokenize, upsert } from './knowledge'
import type { Entry } from './knowledge'
import { addUsage } from './pricing'
import { DEDUPED_TOOLS, IMAGE_READ_DENY, WRITING_TOOLS, countImageRead, duplicateReadDeny, forgetReads, imageLimitReached, isImageRead, pendingReads, readKey, resetImageReads, settleRead } from './reads'
import { GOAL_LABEL_PROMPT, ROUTER_MIN_PROMPT, ROUTER_PROMPT, firstLine, isQuestion, isWellBriefed, parseRoute, truncateGoal, withGoalContext, withReportLimit } from './routing'
import type { Route } from './routing'
import { makePanel } from './panel'
import { firstPrompt, relayTarget, threadLines } from './threadLines'
import { makeThread } from './thread'
import { dismissAgent, selectAgent, selectedAgent } from './select'
import { loadSnapshot, saveSnapshot } from './snapshot'
import { trimResultContent } from './trim'
import { MAIN, buildTree, flatten, isLive, rememberAgents, needsCompact, trackLifecycle, visibleAgents } from './tree'
import { mainTurn, wakeClock } from './clock'

const usageByAgent = atom({ plugin: 'agent-graph', key: 'usage' } as const, {} as Record<string, AgentUsage>)

const activityByAgent = atom({ plugin: 'agent-graph', key: 'activity' } as const, {} as Record<string, string>)

const modelByAgent = atom({ plugin: 'agent-graph', key: 'models' } as const, {} as Record<string, string>)

const routeSourceByAgent = atom({ plugin: 'agent-graph', key: 'routes' } as const, {} as Record<string, string>)

const goalByAgent = atom({ plugin: 'agent-graph', key: 'goals' } as const, {} as Record<string, string>)

const trimmedTokens = atom({ plugin: 'agent-graph', key: 'trimmed' } as const, 0)

const mainFinishedAtom = atom({ plugin: 'agent-graph', key: 'mainFinished' } as const, false)

const storeOf = ($: Parameters<typeof update>[0]) => ({ get: (key: string) => $.store.get(key), set: (key: string, value: unknown) => $.store.set(key, value) })

const persistState = async ($: Parameters<typeof update>[0]) =>
  saveSnapshot(storeOf($), await $.session.id(), {
    usage: await read($, usageByAgent),
    models: await read($, modelByAgent),
    routes: await read($, routeSourceByAgent),
    goals: await read($, goalByAgent),
    trimmed: await read($, trimmedTokens),
    progress: await read($, progressByAgent),
    todos: await read($, todosByAgent),
    mainFinished: await read($, mainFinishedAtom),
  })

const restoreState = async ($: Parameters<typeof update>[0]) => {
  if (Object.keys(await read($, usageByAgent)).length) return
  const snapshot = await loadSnapshot(storeOf($), await $.session.id())
  if (!snapshot) return
  await update($, usageByAgent, () => snapshot.usage as never)
  await update($, modelByAgent, () => snapshot.models as never)
  await update($, routeSourceByAgent, () => snapshot.routes as never)
  await update($, goalByAgent, () => snapshot.goals as never)
  await update($, trimmedTokens, () => snapshot.trimmed as never)
  await update($, progressByAgent, () => snapshot.progress as never)
  await update($, todosByAgent, () => (snapshot.todos ?? {}) as never)
  await update($, mainFinishedAtom, () => snapshot.mainFinished as never)
}

const todosByAgent = atom({ plugin: 'agent-graph', key: 'todos' } as const, {} as Record<string, Todo[]>)

const progressByAgent = atom({ plugin: 'agent-graph', key: 'progress' } as const, {} as Record<string, GoalProgress>)

const TOOL_PREFIX = 'mcp__agent-graph__'

const PANE = 'agent-graph'
const ROW_CHROME = 12
const COMPACT_BANNER_ROWS = 30
const THREAD_CHROME = 8

type Completer = { model: { complete: (request: { model: string; prompt: string; effort: 'low'; maxTokens: number }) => Promise<{ isAnswered: boolean; text?: string }> } }

const summaries = new Map<string, string>()

const summarizeRequest = async ($: Completer, request: string, context?: string) => {
  const cacheKey = `${context ?? ''}\n${request}`
  const cached = summaries.get(cacheKey)
  if (cached) return cached
  const summary = await requestSummary($, request, context)
  summaries.set(cacheKey, summary)
  evictOldest(summaries)
  return summary
}

const requestSummary = async ($: Completer, request: string, context?: string) => {
  const reply = await $.model.complete({ model: 'haiku', prompt: GOAL_LABEL_PROMPT + withGoalContext(request, context), effort: 'low', maxTokens: 60 })
  const label = firstLine(reply.isAnswered && reply.text ? reply.text : '')
  return truncateGoal(label && !isQuestion(label) ? label : (context ?? firstLine(request)))
}

const jevChoice = async ($: JevHost, state: Record<string, unknown>, instructions: string, criteria: Record<string, string>): Promise<JevChoice> => {
  const key = await $.env.get('JEV_API_KEY')
  if (!key) return { note: 'jev: pas de clé' }
  const response = await $.http.fetch(JEV_URL, {
    method: 'POST',
    headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ state, model: 'jev-latest', questions: { q: { type: 'choice', instructions, criteria } } }),
  })
  if (!response.ok) return { note: `jev: HTTP ${response.status}` }
  const answer = JSON.parse(response.text).answers?.q
  if (!(answer?.choice in criteria)) return { note: 'jev: réponse invalide' }
  return answer.confidence >= JEV_MIN_CONFIDENCE
    ? { choice: answer.choice, note: `jev ${percent(answer.confidence)}` }
    : { note: `jev: confiance ${percent(answer.confidence)}` }
}

const jevModel = ($: JevHost, prompt: string) =>
  jevChoice($, { task: prompt.slice(0, JEV_TASK_MAX) }, 'Which agent profile does `task` need?', MODEL_CRITERIA).catch(jevFailure)

const routeSpawn = async ($: Completer & JevHost, prompt: string): Promise<Route | undefined> => {
  const verdict = await jevModel($, prompt)
  if (prompt.length < ROUTER_MIN_PROMPT || isWellBriefed(prompt)) return verdict.choice ? { model: verdict.choice, prompt, source: verdict.note } : undefined
  const reply = await $.model.complete({ model: 'haiku', prompt: ROUTER_PROMPT + prompt, effort: 'low', maxTokens: 1500 })
  const route = reply.isAnswered && reply.text ? parseRoute(reply.text) : undefined
  if (!route) return undefined
  return verdict.choice ? { ...route, model: verdict.choice, source: verdict.note } : { ...route, source: `haiku (${verdict.note})` }
}

const CHEAP_FAMILY = 'haiku'
const CHEAP_EFFORT = 'low'

const noteFailure = async ($: JevHost, key: string, output: string) => {
  const attempts = recordFailure(key, output.slice(0, JEV_TASK_MAX))
  if (attempts.length < BREAKER_FAILURES) return
  const verdict = await jevChoice($, { attempts }, 'Given `attempts`, should the agent retry or stop?', RETRY_OR_STOP).catch(jevFailure)
  if (verdict.choice !== 'stop') return
  stopped.add(key)
  evictOldest(stopped)
}

const setGoal = async ($: Completer & Parameters<typeof update>[0], agent: string, request: string) => {
  const context = (await read($, goalByAgent))[MAIN]
  update($, goalByAgent, all => ({ ...all, [agent]: all[agent] ?? truncateGoal(firstLine(request)) })).catch(() => undefined)
  const goal = await summarizeRequest($, request, context)
  await update($, goalByAgent, all => ({ ...all, [agent]: goal }))
}


const DEFAULT_COLS = 60

type ClockHost = { ui: { invalidate: (event: 'ui.render') => void }; agent: { list: () => Promise<readonly { status: string }[]> } }

const wakeAnimation = ($: ClockHost) =>
  wakeClock(() => $.ui.invalidate('ui.render'), async () => (await $.agent.list()).some(({ status }) => isLive(status)))

const loadKnowledge = async ($: Parameters<typeof update>[0]) => ((await $.store.get(STORE_KEY)) as Entry[] | undefined) ?? []

const saveEntry = async ($: Parameters<typeof update>[0], entry: Entry) => {
  const entries = upsert(await loadKnowledge($), entry)
  hydrateMemories(entries)
  await $.store.set(STORE_KEY, entries)
}

const COMPACTION_LABEL = 'compaction'

const learn = async ($: Parameters<typeof update>[0], goal: string, report: string | undefined) => {
  if (!report) return
  const tags = [...new Set(tokenize(goal))].slice(0, MAX_LEARNED_TAGS)
  await saveEntry($, { key: goal.slice(0, LEARNED_KEY_CHARS), text: clip(report.replace(SECRET_PATTERN, SECRET_MASK)), tags, at: Date.now(), learned: true })
}

type BriefHost = Completer & Parameters<typeof update>[0]

const buildBrief = (text: string) => memoryBrief(relevantMemories(text))

type Submission = { text: string; turnId?: string; origin: { kind: string } }

const isComposerPrompt = (e: Submission) => !e.text.startsWith('/') && !e.turnId && e.origin.kind === 'composer'

const isBriefable = async ($: BriefHost, e: Submission) => isComposerPrompt(e) && !!(await $.store.get(BRIEF_KEY))

const briefFor = async ($: BriefHost, e: Submission) => ((await isBriefable($, e)) ? buildBrief(e.text) : undefined)

const modeWanted = async ($: BriefHost, e: Submission) => isComposerPrompt(e) && looksMultiStep(e.text) && !!(await $.store.get(MODE_KEY))

const modeFor = async ($: BriefHost & JevHost, e: Submission) => {
  const verdict = await jevChoice($, { task: e.text.slice(0, JEV_TASK_MAX) }, 'Which working mode does `task` need?', MODE_CRITERIA)
  return modeAdvice(verdict.choice)
}

type PlanHost = Completer & Parameters<typeof update>[0] & { session: { messages: () => Promise<unknown> } }

const mainContext = { tokens: 0 }

const projectDir: { cwd?: string } = {}

const recentMessages = async ($: PlanHost) => {
  const messages = await $.session.messages()
  return Array.isArray(messages) ? recentContext(messages) : ''
}

const planRequest = async ($: PlanHost, request: string): Promise<Plan> => {
  const goal = (await read($, goalByAgent))[MAIN]
  const context = [goal && `Objectif en cours: ${goal}`, await recentMessages($)].filter(Boolean).join('\n\n')
  const reply = await $.model.complete({ model: 'haiku', prompt: FLOW_PROMPT + planInput(request, context), effort: 'low', maxTokens: FLOW_MAX_TOKENS })
  return reply.isAnswered && reply.text ? parsePlan(reply.text) : { features: [] }
}

const mainState = async ($: Parameters<typeof update>[0]) => {
  const usage = (await read($, usageByAgent))[MAIN]
  return { tokens: mainContext.tokens, model: usage?.model, cacheHitRate: usage ? cacheHitRate(usage) : 0 }
}

const startFlow = async ($: PlanHost, actions: FlowActions, e: Submission) => {
  const plan = await withTimeout(planRequest($, e.text), FLOW_TIMEOUT_MS)
  if (!plan) return
  if (plan.goal) void update($, goalByAgent, all => ({ ...all, [MAIN]: truncateGoal(plan.goal as string) }))
  stashFlow(await runFlow(actions, plan.features, await mainState($)))
}

const flowEnabled = async ($: BriefHost, e: Submission) => isComposerPrompt(e) && looksMultiStep(e.text) && !!(await $.store.get(FLOW_KEY))

const autoCompact = async ($: Parameters<typeof update>[0] & { session: { compact: (args: { instructions: string }) => Promise<unknown> } }, tokens: number) => {
  if (tokens < AUTO_COMPACT_TOKENS) autoCompactState.armed = true
  const due = autoCompactState.armed && tokens >= AUTO_COMPACT_TOKENS && (await read($, mainFinishedAtom)) && !!(await $.store.get(AUTO_COMPACT_KEY))
  if (!due) return
  autoCompactState.armed = false
  const goal = (await read($, goalByAgent))[MAIN]
  await $.session.compact({ instructions: goal ? `${AUTO_COMPACT_KEEP}. Current goal: ${goal}` : AUTO_COMPACT_KEEP }).catch(error => {
    autoCompactState.armed = true
    throw error
  })
}

const SWITCHES = new Map([
  ['flow', { key: FLOW_KEY, label: 'Flow parallèle automatique' }],
  ['brief', { key: BRIEF_KEY, label: "Brief d'orientation" }],
  ['mode', { key: MODE_KEY, label: 'Mode conseillé' }],
  ['compact', { key: AUTO_COMPACT_KEY, label: 'Compactage automatique' }],
])

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({ name: 'agent-graph', description: 'Show running agents with tokens and estimated cost', argumentHint: '[brief|mode|compact|flow on|off]' })
    void $.ui.open({ id: PANE, title: 'Agents' })
    wakeAnimation($)
    resetImageReads()
    resetBreaker()
    resetNudge(await $.session.id())
    projectDir.cwd = (e as { cwd?: string }).cwd
    await restoreState($).catch(() => undefined)
    hydrateMemories(await loadKnowledge($))
    await $.tool.register(REMEMBER_TOOL)
    await $.tool.register(RECALL_TOOL)
    await $.tool.register(PROGRESS_TOOL)
    return next(e)
  })

  on('command.run', { command: 'agent-graph' }, async ($, e) => {
    const [subcommand, rawValue] = e.args.trim().split(/\s+/)
    const value = rawValue?.toLowerCase()
    const toggle = SWITCHES.get(subcommand)
    if (toggle && (value === 'on' || value === 'off')) {
      await $.store.set(toggle.key, value === 'on')
      return { text: `${toggle.label} ${value === 'on' ? 'activé' : 'désactivé'}.` }
    }
    if (toggle) return { text: `${toggle.label}: /agent-graph ${subcommand} on|off` }
    await $.ui.open({ id: PANE, title: 'Agents' })
    return { text: 'Agent graph opened.' }
  })

  on('session.measure', async ($, e, next) => {
    if (e.context.tokens !== undefined) mainContext.tokens = e.context.tokens
    if (e.context.tokens !== undefined) void autoCompact($, e.context.tokens).catch(() => undefined)
    return next(e)
  })

  on('tool.call', async ($, e, next) => {
    const agent = e.agentId ?? MAIN
    wakeAnimation($)
    const activity = describeCall(e as Record<string, unknown>)
    void update($, activityByAgent, all => ({ ...all, [agent]: activity }))
    if (e.tool === 'TodoWrite' && e.todos.length) {
      void update($, progressByAgent, all => ({ ...all, [agent]: todoProgress(e.todos) }))
      void update($, todosByAgent, all => ({ ...all, [agent]: toTodos(e.todos) }))
    }
    if (e.tool === 'TaskCreate') void update($, progressByAgent, all => ({ ...all, [agent]: countTask(all[agent], { total: 1 }) }))
    if (e.tool === 'TaskUpdate' && e.status === TASK_STATUS_DONE) void update($, progressByAgent, all => ({ ...all, [agent]: countTask(all[agent], { done: 1 }) }))
    const key = `${e.tool}:${activity}`
    startCall()
    if (stopped.has(key)) return { deny: BREAKER_DENY }
    if (isImageRead(e as Record<string, unknown>) && imageLimitReached()) return { deny: IMAGE_READ_DENY }
    if (DEDUPED_TOOLS.includes(e.tool)) {
      const deny = duplicateReadDeny(agent, e as Record<string, unknown>)
      if (deny) return { deny }
    }
    const result = await next(e)
    if (WRITING_TOOLS.includes(e.tool) || (e.tool === 'Bash' && !result.isReadOnly)) forgetReads()
    if (result.isError) await noteFailure($, key, String(result.text ?? 'error')).catch(() => undefined)
    else recordSuccess(key)
    const succeeded = !result.isError && !result.deny
    if (succeeded && isImageRead(e as Record<string, unknown>)) countImageRead()
    if (succeeded && e.tool === 'Agent') await learn($, `${e.description}: ${e.prompt}`, result.text).catch(() => undefined)
    if (succeeded && DEDUPED_TOOLS.includes(e.tool)) pendingReads.set(e.tool_use_id, readKey(agent, e as Record<string, unknown>))
    return result
  })

  on('session.append', { door: 'tool-result' }, async ($, e, next) => {
    let savedTokens = 0
    const content = e.message.content.map(block => {
      if (block.type !== 'tool_result') return block
      let saved = 0
      const trimmed = { ...block, content: trimResultContent(block.content, tokens => (saved += tokens)) }
      settleRead(block.tool_use_id, !block.is_error && !saved)
      savedTokens += saved
      return trimmed
    })
    if (savedTokens) void update($, trimmedTokens, total => total + savedTokens)
    return next({ ...e, message: { ...e.message, content } })
  })

  on('tool.call', { tool: `${TOOL_PREFIX}remember` }, async ($, e) => {
    const parsed = parseRemember(e as unknown as { key?: string; text?: string; tags?: string[] })
    if ('error' in parsed) return { result: parsed.error }
    await saveEntry($, { ...parsed.entry, at: Date.now() })
    return { result: `Enregistré: ${parsed.entry.key}` }
  })

  on('tool.call', { tool: `${TOOL_PREFIX}recall` }, async ($, e) => {
    const { query = '', key } = e as unknown as { query?: string; key?: string }
    const entries = await loadKnowledge($)
    const exact = key && entries.find(entry => entry.key === key)
    return { result: exact ? exact.text : search(entries, query || key || '') }
  })

  on('tool.call', { tool: `${TOOL_PREFIX}progress` }, async ($, e) => {
    const { done, total, steps } = e as unknown as GoalProgress & { steps?: string[] }
    const agent = e.agentId ?? MAIN
    void update($, progressByAgent, all => ({ ...all, [agent]: { done, total } }))
    if (steps?.length) void update($, todosByAgent, all => ({ ...all, [agent]: stepsToTodos(steps, done) }))
    return { result: `${done}/${total}` }
  })

  on('prompt.compose', async ($, e, next) => {
    const { sections } = await next(e)
    return { sections: [...sections, KNOWLEDGE_SECTION, PROGRESS_SECTION] }
  })

  on('session.compact', async ($, e, next) => {
    forgetReads()
    const result = await next(e)
    if (result.messages && !e.agentId && e.trigger !== 'precompute') {
      const goal = (await read($, goalByAgent))[MAIN] ?? COMPACTION_LABEL
      await learn($, goal, result.messages[0]?.text).catch(() => undefined)
    }
    return result
  })

  on('turn.start', async ($, e, next) => {
    if (!e.agentId) {
      mainTurn.busy = true
      void update($, mainFinishedAtom, () => false)
    }
    wakeAnimation($)
    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    if (!e.agentId) {
      mainTurn.busy = false
      void update($, mainFinishedAtom, () => true)
    }
    void persistState($).catch(() => undefined)
    return next(e)
  })

  on('prompt.submit', async ($, e, next) => {
    const started = Date.now()
    const isCommand = e.text.startsWith('/')
    const flowWanted = await flowEnabled($, e)
    const modeAsked = await modeWanted($, e)
    const earlierFlow = isCommand ? undefined : takeFlow()
    if (!isCommand) {
      void update($, goalByAgent, all => ({ ...all, [MAIN]: truncateGoal(firstLine(e.text)) }))
      void update($, progressByAgent, ({ [MAIN]: _previous, ...others }) => others)
      void update($, todosByAgent, ({ [MAIN]: _previous, ...others }) => others)
    }
    if (flowWanted) void startFlow($, { explore: input => $.tool.call(input), spawn: input => $.agent.spawn(input) }, e).catch(() => undefined)
    const [brief, mode] = await Promise.all([briefFor($, e).catch(() => undefined), modeAsked ? withTimeout(modeFor($, e), BRIEF_TIMEOUT_MS).catch(() => undefined) : undefined])
    const session = await $.session.id()
    const added = [onlyIfChanged(session, 'brief', brief), onlyIfChanged(session, 'mode', mode), earlierFlow, isCommand ? undefined : contextNudge(mainContext.tokens, session)].filter((text): text is string => !!text)
    recordPrompt({ hookLatencyMs: Date.now() - started, injectedChars: added.reduce((sum, text) => sum + text.length, 0), modelCalls: Number(flowWanted) + Number(modeAsked) }, projectDir.cwd)
    return next(added.length ? { ...e, context: [...(e.context ?? []), ...added] } : e)
  })

  on('agent.spawn', async ($, e, next) => {
    const route = await routeSpawn($, e.prompt).catch(() => undefined)
    const routed = route ? { ...e, prompt: route.prompt, ...(e.model ? {} : { model: route.model }) } : e
    const result = await next({ ...routed, prompt: withReportLimit(routed.prompt) })
    if (result.agentId) {
      const { agentId } = result
      const model = result.model ?? route?.model
      if (model) void update($, modelByAgent, all => ({ ...all, [agentId]: model }))
      if (route) void update($, routeSourceByAgent, all => ({ ...all, [agentId]: route.source }))
      void setGoal($, agentId, route?.prompt ?? e.prompt).catch(() => undefined)
    }
    return result
  })

  on('turn.step', async function* ($, e, next) {
    const model = e.agentId ? (await read($, modelByAgent))[e.agentId] : undefined
    const response = yield* next(model?.includes(CHEAP_FAMILY) ? { ...e, effort: CHEAP_EFFORT } : e)
    if (response.usage) {
      const { usage } = response
      await update($, usageByAgent, all => ({ ...all, [e.agentId ?? MAIN]: addUsage(all[e.agentId ?? MAIN], usage) }))
    }
    return response
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const ui = $.ui.resolve(e)
    const { Box, Text } = ui
    const usage = await read($, usageByAgent)
    const activity = await read($, activityByAgent)
    const goals = await read($, goalByAgent)
    const progress = await read($, progressByAgent)
    const todos = await read($, todosByAgent)
    const models = await read($, modelByAgent)
    const routes = await read($, routeSourceByAgent)
    const now = Date.now()
    const agents = rememberAgents(await $.agent.list())
    trackLifecycle(agents, now)
    const root = buildTree(agents, usage, activity, goals, progress, todos, models, await read($, mainFinishedAtom), now)
    const allRows = flatten(root)
    const maxRows = Math.max(1, (e.viewport?.rows ?? 24) - ROW_CHROME)
    const { shown: rows, hidden } = visibleAgents(allRows, maxRows)
    const session = await $.session.usage({ breakdown: 'summary' })
    const quotas = mostUrgent(rateBars(session.rateLimits))
    const { breakdown } = session.context
    const limit = breakdown ? compactionLimit(breakdown) : session.context.window
    const contextPercent = limit && session.context.tokens !== undefined ? clampedPercent((session.context.tokens / limit) * 100) : clampedPercent(session.context.percent)
    const contextUsage = contextSummary({ tokens: session.context.tokens, window: limit })
    const segments = breakdown ? contextSegments(breakdown.categories, limit) : []

    const { Section, Quota, AgentCard, Sprite } = makePanel(ui)
    const { ContextBreakdown, ContextFigure } = makeContext(ui)
    const { Banner } = makeBanner(ui)
    const cols = e.viewport?.columns ?? DEFAULT_COLS
    const open = (id: string | undefined) => {
      selectAgent(id)
      $.ui.invalidate('ui.render')
    }
    const dismiss = (id: string) => {
      dismissAgent(id)
      $.ui.invalidate('ui.render')
    }
    const target = flatten(root).find(({ node }) => node.id === selectedAgent())?.node
    if (target) {
      const { Thread } = makeThread(ui as never)
      const messages = await $.session.messages({ agentId: target.id })
      const rows = Array.isArray(messages) ? messages : []
      const lines = threadLines(rows, Math.max(1, (e.viewport?.rows ?? 24) - THREAD_CHROME))
      const send = (text: string) => {
        const message = relayTarget(target.id, text)
        if (message) void $.session.send(message)
      }
      return <Thread title={target.label} stats={target.usage ? usageLines(target.usage) : []} prompt={firstPrompt(rows)} avatar={<Sprite node={target} />} lines={lines} cols={cols} onBack={() => open(undefined)} onSend={send} />
    }

    return (
      <Box flexDirection="column" padding={1}>
        <Banner compact={(e.viewport?.rows ?? 24) < COMPACT_BANNER_ROWS} />
        <Section title="CONTEXTE" icon={ICONS.context} aside={<ContextFigure percent={contextPercent} />}>
          <ContextBreakdown percent={contextPercent} usage={contextUsage} segments={segments} />
        </Section>
        <Section title="QUOTAS" icon={ICONS.quotas}>
          {quotas.map(quota => <Quota {...quota} />)}
        </Section>
        <Section title="AGENTS" icon={ICONS.agents}>
          {rows.map(({ node, depth, last, ancestorsLast }) => <AgentCard node={node} depth={depth} last={last} ancestorsLast={ancestorsLast} compact={needsCompact(allRows, maxRows)} route={routes[node.id]} onOpen={open} onDismiss={dismiss} />)}
          {hidden > 0 && <Text dimColor>{`+${hidden}`}</Text>}
        </Section>
      </Box>
    )
  })
}
