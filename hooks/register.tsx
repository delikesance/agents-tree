import { atom, read, update } from 'claude-code'
import type { CoreEngineInterface, Register, StateDollar } from 'claude-code'

import type { AgentUsage, GoalProgress } from '../types'

import { PROGRESS_SECTION, PROGRESS_TOOL } from './progress'
import type { Todo } from './activity'
import { TASK_STATUS_DONE, countTask, describeCall, stepsToTodos, todoProgress, toTodos } from './activity'
import { BREAKER_DENY, BREAKER_FAILURES, RETRY_OR_STOP, evictOldest, recordFailure, recordSuccess, resetBreaker, startCall, stopped } from './breaker'
import { stashFlow, takeFlow } from './backgroundFlow'
import { BRIEF_KEY, BRIEF_TIMEOUT_MS, memoryBrief, withTimeout } from './brief'
import { AUTO_COMPACT_KEEP, AUTO_COMPACT_KEY, AUTO_COMPACT_TOKENS, autoCompactState, contextNudge, resetNudge } from './compact'
import { declaredModel, modelToInject } from './declaredModel'
import { FLOW_KEY, FLOW_MAX_TOKENS, FLOW_PROMPT, FLOW_TIMEOUT_MS, looksMultiStep, parsePlan, planInput, recentContext } from './flow'
import type { Plan } from './flow'
import { runFlow } from './flowRun'
import type { FlowActions } from './flowRun'
import { recordPrompt } from './metrics'
import { relevantMemories } from './recall-inject'
import { parseRemember } from './remember'
import { onlyIfChanged } from './injected'
import { asksUltracode, ultracodeNote } from './ultracode'
import { makeBanner } from './banner'
import { makeContext } from './contextSection'
import { cacheHitRate, clampedPercent, compactionLimit, contextSegments, contextSummary, percent, mostUrgent, rateBars, usageLines } from './format'
import { ICONS } from './icons'
import { MODE_CRITERIA, MODE_KEY, modeAdvice } from './mode'
import { JEV_MIN_CONFIDENCE, JEV_TASK_MAX, JEV_URL, MODEL_CRITERIA, jevFailure } from './jev'
import type { JevChoice, JevHost } from './jev'
import { isLearnable } from './learnable'
import { KNOWLEDGE_SECTION, LEARNED_KEY_CHARS, MAX_LEARNED_TAGS, RECALL_TOOL, REMEMBER_TOOL, SECRET_MASK, SECRET_PATTERN, STORE_KEY, clip, hydrateMemories, search, tokenize, upsert } from './knowledge'
import type { Entry } from './knowledge'
import { addUsage } from './pricing'
import { DEDUPED_TOOLS, IMAGE_READ_DENY, WRITING_TOOLS, countImageRead, duplicateReadDeny, forgetReads, imageLimitReached, isImageRead, pendingReads, readKey, resetImageReads, settleRead } from './reads'
import { GOAL_LABEL_PROMPT, firstLine, isQuestion, truncateGoal, withGoalContext, withReportLimit } from './routing'
import { takeMapNote } from './mapEnsure'
import { pointAtMap, startMapUpkeep } from './mapWiring'
import type { Route } from './routing'
import { makePanel } from './panel'
import { skillDepsOf, type SkillHostSource } from './skillHost'
import { createSkillLearning } from './skillLearning'
import { makeSkillsSection } from './skillsSection'
import { firstPrompt, relayTarget, threadLines } from './threadLines'
import { makeThread } from './thread'
import { dismissAgent, selectAgent, selectedAgent } from './select'
import { loadSnapshot, saveSnapshot } from './snapshot'
import { trimResultContent } from './trim'
import { MAIN, buildTree, flatten, isLive, rememberAgents, needsCompact, trackLifecycle, visibleAgents } from './tree'
import { finishWorkflowAgent, recordWorkflowAgent, recordWorkflowLaunch, workflowAgents, workflowLaunches } from './workflowRuns'
import { mainTurn, wakeClock } from './clock'

const usageByAgent = atom({ plugin: 'hive', key: 'usage' } as const, {} as Record<string, AgentUsage>)

const activityByAgent = atom({ plugin: 'hive', key: 'activity' } as const, {} as Record<string, string>)

const modelByAgent = atom({ plugin: 'hive', key: 'models' } as const, {} as Record<string, string>)

const routeSourceByAgent = atom({ plugin: 'hive', key: 'routes' } as const, {} as Record<string, string>)

const goalByAgent = atom({ plugin: 'hive', key: 'goals' } as const, {} as Record<string, string>)

const trimmedTokens = atom({ plugin: 'hive', key: 'trimmed' } as const, 0)

const mainFinishedAtom = atom({ plugin: 'hive', key: 'mainFinished' } as const, false)

type StateHost = StateDollar & Pick<CoreEngineInterface, 'store' | 'session'>

const storeOf = ($: StateHost) => ({ get: (key: string) => $.store.get(key), set: (key: string, value: unknown) => $.store.set(key, value) })

type MapHostSource = Pick<CoreEngineInterface, 'process' | 'fs' | 'model' | 'session'>

const fsOf = ($: Pick<MapHostSource, 'fs'>) => ({ read: (path: string) => $.fs.read(path), write: (path: string, text: string) => $.fs.write(path, text), exists: (path: string) => $.fs.exists(path) })

const mapHostOf = ($: MapHostSource): Parameters<typeof startMapUpkeep>[0] => ({ process: { run: (argv, init) => $.process.run(argv, init) }, fs: fsOf($), model: { complete: request => $.model.complete(request) }, session: { id: () => $.session.id() } })

const persistState = async ($: StateHost) =>
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

const restoreState = async ($: StateHost) => {
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

const todosByAgent = atom({ plugin: 'hive', key: 'todos' } as const, {} as Record<string, Todo[]>)

const progressByAgent = atom({ plugin: 'hive', key: 'progress' } as const, {} as Record<string, GoalProgress>)

const TOOL_PREFIX = 'mcp__hive__'

const PANE = 'hive'
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

const JEV_TIMEOUT_MS = 5000
const CLASSIFIER_OUTAGE = 'auto mode classifier gave no verdict'

const jevChoice = async ($: JevHost, state: Record<string, unknown>, instructions: string, criteria: Record<string, string>): Promise<JevChoice> => {
  const key = await $.env.get('JEV_API_KEY')
  if (!key) return { note: 'jev: pas de clé' }
  const response = await withTimeout($.http.fetch(JEV_URL, {
    method: 'POST',
    headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ state, model: 'jev-latest', questions: { q: { type: 'choice', instructions, criteria } } }),
  }), JEV_TIMEOUT_MS)
  if (!response) return { note: 'jev: timeout' }
  if (!response.ok) return { note: `jev: HTTP ${response.status}` }
  const answer = JSON.parse(response.text).answers?.q
  if (!(answer?.choice in criteria)) return { note: 'jev: réponse invalide' }
  return answer.confidence >= JEV_MIN_CONFIDENCE
    ? { choice: answer.choice, note: `jev ${percent(answer.confidence)}` }
    : { note: `jev: confiance ${percent(answer.confidence)}` }
}

const jevModel = ($: JevHost, prompt: string) =>
  jevChoice($, { task: prompt.slice(0, JEV_TASK_MAX) }, 'Which agent profile does `task` need?', MODEL_CRITERIA).catch(jevFailure)

const routeSpawn = async ($: JevHost, prompt: string): Promise<Route | undefined> => {
  const verdict = await jevModel($, prompt)
  return verdict.choice ? { model: verdict.choice, source: verdict.note } : undefined
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

const setGoal = async ($: Completer & StateHost, agent: string, request: string) => {
  const context = (await read($, goalByAgent))[MAIN]
  update($, goalByAgent, all => ({ ...all, [agent]: all[agent] ?? truncateGoal(firstLine(request)) })).catch(() => undefined)
  const goal = await summarizeRequest($, request, context)
  await update($, goalByAgent, all => ({ ...all, [agent]: goal }))
}


const DEFAULT_COLS = 60

type ClockHost = { ui: { invalidate: (event: 'ui.render') => void }; agent: { list: () => Promise<readonly { status: string }[]> } }

const wakeAnimation = ($: ClockHost) =>
  wakeClock(() => $.ui.invalidate('ui.render'), async () => (await $.agent.list()).some(({ status }) => isLive(status)))

const loadKnowledge = async ($: StateHost) => (((await $.store.get(STORE_KEY)) as Entry[] | undefined) ?? []).filter(entry => !entry.learned || isLearnable(entry.text))

const saveEntry = async ($: StateHost, entry: Entry) => {
  const entries = upsert(await loadKnowledge($), entry)
  hydrateMemories(entries)
  await $.store.set(STORE_KEY, entries)
}

type ServedCall = Record<string, unknown> & { agentId?: string }

const serveRemember = async ($: StateHost, e: ServedCall) => {
  const parsed = parseRemember(e as { key?: string; text?: string; tags?: string[] })
  if ('error' in parsed) return { result: parsed.error }
  await saveEntry($, { ...parsed.entry, at: Date.now() })
  return { result: `Enregistré: ${parsed.entry.key}` }
}

const serveRecall = async ($: StateHost, e: ServedCall) => {
  const { query = '', key } = e as { query?: string; key?: string }
  const entries = await loadKnowledge($)
  const exact = key && entries.find(entry => entry.key === key)
  return { result: exact ? exact.text : search(entries, query || key || '') }
}

const serveProgress = async ($: StateHost, e: ServedCall) => {
  const { done, total, steps } = e as unknown as GoalProgress & { steps?: string[] }
  const agent = e.agentId ?? MAIN
  void update($, progressByAgent, all => ({ ...all, [agent]: { done, total } }))
  if (steps?.length) void update($, todosByAgent, all => ({ ...all, [agent]: stepsToTodos(steps, done) }))
  return { result: `${done}/${total}` }
}

const COMPACTION_LABEL = 'compaction'

const learn = async ($: StateHost, goal: string, report: string | undefined) => {
  if (!report || !isLearnable(report)) return
  const tags = [...new Set(tokenize(goal))].slice(0, MAX_LEARNED_TAGS)
  await saveEntry($, { key: goal.slice(0, LEARNED_KEY_CHARS), text: clip(report.replace(SECRET_PATTERN, SECRET_MASK)), tags, at: Date.now(), learned: true })
}

type BriefHost = Completer & StateHost

const buildBrief = (text: string) => memoryBrief(relevantMemories(text))

type Submission = { text: string; turnId?: string; origin: { kind: string } }

const isTaskNotification = (e: Submission) => e.text.startsWith('<task-notification>')
const isComposerPrompt = (e: Submission) => !e.text.startsWith('/') && !e.turnId && e.origin.kind === 'composer'

const isBriefable = async ($: BriefHost, e: Submission) => isComposerPrompt(e) && !!(await $.store.get(BRIEF_KEY))

const briefFor = async ($: BriefHost, e: Submission) => ((await isBriefable($, e)) ? buildBrief(e.text) : undefined)

const isOwnMultiStep = (e: Submission) => isComposerPrompt(e) && looksMultiStep(e.text) && !asksUltracode(e.text)

const modeWanted = async ($: BriefHost, e: Submission) => isOwnMultiStep(e) && !!(await $.store.get(MODE_KEY))

const modeFor = async ($: BriefHost & JevHost, e: Submission) => {
  const verdict = await jevChoice($, { task: e.text.slice(0, JEV_TASK_MAX) }, 'Which working mode does `task` need?', MODE_CRITERIA)
  return modeAdvice(verdict.choice)
}

type PlanHost = Completer & StateHost & { session: { messages: () => Promise<unknown> } }

const mainContext = { tokens: 0 }

const projectDir: { cwd?: string } = {}

const learning = createSkillLearning()

const skillsOf = ($: SkillHostSource) =>
  skillDepsOf(
    {
      store: { get: key => $.store.get(key), set: (key, value) => $.store.set(key, value) },
      fs: { read: path => $.fs.read(path), write: (path, text) => $.fs.write(path, text), exists: path => $.fs.exists(path) },
      process: { run: (argv, init) => $.process.run(argv, init) },
      session: { id: () => $.session.id() },
    },
    projectDir.cwd,
  )

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

const mainState = async ($: StateHost) => {
  const usage = (await read($, usageByAgent))[MAIN]
  return { tokens: mainContext.tokens, model: usage?.model, cacheHitRate: usage ? cacheHitRate(usage) : 0 }
}

const startFlow = async ($: PlanHost, actions: FlowActions, e: Submission) => {
  const plan = await withTimeout(planRequest($, e.text), FLOW_TIMEOUT_MS)
  if (!plan) return
  if (plan.goal) void update($, goalByAgent, all => ({ ...all, [MAIN]: truncateGoal(plan.goal as string) }))
  stashFlow(await runFlow(actions, plan.features, await mainState($)))
}

const flowEnabled = async ($: BriefHost, e: Submission) => isOwnMultiStep(e) && !!(await $.store.get(FLOW_KEY))

const autoCompact = async ($: StateHost & { session: { compact: (args: { instructions: string }) => Promise<unknown> } }, tokens: number) => {
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
    await $.command.register({ name: 'hive', description: 'HIVE: show running agents with tokens and estimated cost', argumentHint: '[id|brief|mode|compact|discover [apply]|flow on|off]' })
    void $.ui.open({ id: PANE, title: 'HIVE' })
    wakeAnimation($)
    resetImageReads()
    resetBreaker()
    resetNudge(await $.session.id())
    projectDir.cwd = (e as { cwd?: string }).cwd
    await restoreState($).catch(() => undefined)
    hydrateMemories(await loadKnowledge($))
    void learning.onSessionStart(skillsOf($))
    await $.tool.register(REMEMBER_TOOL)
    await $.tool.register(RECALL_TOOL)
    await $.tool.register(PROGRESS_TOOL)
    return next(e)
  })

  on('command.run', { command: 'hive' }, async ($, e) => {
    const [subcommand, rawValue] = e.args.trim().split(/\s+/)
    if (subcommand === 'id') return { text: `Conversation ID: ${await $.session.id()}` }
    const value = rawValue?.toLowerCase()
    if (subcommand === 'discover') return { text: await learning.discover(skillsOf($), value === 'apply') }
    const toggle = SWITCHES.get(subcommand ?? '')
    if (toggle && (value === 'on' || value === 'off')) {
      await $.store.set(toggle.key, value === 'on')
      return { text: `${toggle.label} ${value === 'on' ? 'activé' : 'désactivé'}.` }
    }
    if (toggle) return { text: `${toggle.label}: /hive ${subcommand} on|off` }
    await $.ui.open({ id: PANE, title: 'HIVE' })
    return { text: 'HIVE opened.' }
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
    if (e.tool === 'Workflow' && !e.agentId) recordWorkflowLaunch(Date.now())
    if (e.tool === 'TaskCreate') void update($, progressByAgent, all => ({ ...all, [agent]: countTask(all[agent], { total: 1 }) }))
    if (e.tool === 'TaskUpdate' && e.status === TASK_STATUS_DONE) void update($, progressByAgent, all => ({ ...all, [agent]: countTask(all[agent], { done: 1 }) }))
    if (e.tool === `${TOOL_PREFIX}remember`) return serveRemember($, e as ServedCall)
    if (e.tool === `${TOOL_PREFIX}recall`) return serveRecall($, e as ServedCall)
    if (e.tool === `${TOOL_PREFIX}progress`) return serveProgress($, e as ServedCall)
    const key = `${agent}:${e.tool}:${activity}`
    startCall()
    if (stopped.has(key)) return { deny: BREAKER_DENY }
    if (isImageRead(e as Record<string, unknown>) && imageLimitReached()) return { deny: IMAGE_READ_DENY }
    if (DEDUPED_TOOLS.includes(e.tool)) {
      const deny = duplicateReadDeny(agent, e as Record<string, unknown>)
      if (deny) return { deny }
    }
    const result = await next(e)
    if (WRITING_TOOLS.includes(e.tool) || (e.tool === 'Bash' && !result.isReadOnly)) forgetReads()
    const failure = result.isError ? String(result.text ?? 'error') : undefined
    if (failure === undefined) recordSuccess(key)
    else if (!failure.includes(CLASSIFIER_OUTAGE)) await noteFailure($, key, failure).catch(() => undefined)
    const succeeded = !result.isError && !result.deny
    if (!e.agentId) void learning.onTool(skillsOf($), e as Record<string, unknown> & { tool: string }, !succeeded)
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
      if (typeof block.tool_use_id === 'string') settleRead(block.tool_use_id, !block.is_error && !saved)
      savedTokens += saved
      return trimmed
    })
    if (savedTokens) void update($, trimmedTokens, total => total + savedTokens)
    return next({ ...e, message: { ...e.message, content } })
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
    mainTurn.busy = true
    void update($, mainFinishedAtom, () => false)
    wakeAnimation($)
    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    if (!e.agentId) {
      mainTurn.busy = false
      void update($, mainFinishedAtom, () => true)
      void learning.onTurnComplete(skillsOf($))
    }
    if (e.agentId) finishWorkflowAgent(e.agentId, Date.now())
    void persistState($).catch(() => undefined)
    return next(e)
  })

  on('skill.prompt', async ($, e, next) => {
    void learning.onSkillPrompt(skillsOf($), e.skill)
    return next(e)
  })

  on('prompt.submit', async ($, e, next) => {
    const started = Date.now()
    const isCommand = e.text.startsWith('/')
    const flowWanted = await flowEnabled($, e)
    const modeAsked = await modeWanted($, e)
    const earlierFlow = isCommand ? undefined : takeFlow()
    const mapNote = isCommand ? undefined : takeMapNote()
    startMapUpkeep(mapHostOf($), projectDir.cwd, isCommand)
    if (!isCommand && !isTaskNotification(e)) {
      void learning.onPrompt(skillsOf($), e.text)
      void update($, goalByAgent, all => ({ ...all, [MAIN]: truncateGoal(firstLine(e.text)) }))
      void update($, progressByAgent, ({ [MAIN]: _previous, ...others }) => others)
      void update($, todosByAgent, ({ [MAIN]: _previous, ...others }) => others)
    }
    if (flowWanted) void startFlow($, { explore: input => $.tool.call(input), spawn: input => $.agent.spawn(input) }, e).catch(() => undefined)
    const [brief, mode] = await Promise.all([briefFor($, e).catch(() => undefined), modeAsked ? withTimeout(modeFor($, e), BRIEF_TIMEOUT_MS).catch(() => undefined) : undefined])
    const session = await $.session.id()
    const added = [onlyIfChanged(session, 'brief', brief), onlyIfChanged(session, 'mode', mode), isComposerPrompt(e) ? ultracodeNote(e.text) : undefined, earlierFlow, onlyIfChanged(session, 'map', mapNote), isCommand ? undefined : contextNudge(mainContext.tokens, session)].filter((text): text is string => !!text)
    if (projectDir.cwd) void recordPrompt(fsOf($), { hookLatencyMs: Date.now() - started, injectedChars: added.reduce((sum, text) => sum + text.length, 0), modelCalls: Number(flowWanted) + Number(modeAsked) }, projectDir.cwd)
    return next(added.length ? { ...e, context: [...(e.context ?? []), ...added] } : e)
  })

  on('agent.spawn', async ($, e, next) => {
    const declared = e.workflow ? undefined : await declaredModel(path => $.fs.read(path), [projectDir.cwd, await $.env.get('HOME')], e.subagentType).catch(() => undefined)
    const route = e.workflow || e.model || declared ? undefined : await routeSpawn($, e.prompt).catch(() => undefined)
    const injected = route ? modelToInject(e.model, declared, route.model) : undefined
    const routed = injected ? { ...e, model: injected } : e
    // The host ignores any rewrite of a workflow agent and logs it as a plugin failure.
    const result = await next(e.workflow ? e : { ...routed, prompt: await pointAtMap(mapHostOf($), projectDir.cwd, withReportLimit(routed.prompt)) })
    if (result.agentId) {
      const { agentId } = result
      const model = result.model ?? injected ?? declared
      if (model) void update($, modelByAgent, all => ({ ...all, [agentId]: model }))
      if (route) void update($, routeSourceByAgent, all => ({ ...all, [agentId]: route.source }))
      void setGoal($, agentId, e.prompt).catch(() => undefined)
      if (e.workflow) recordWorkflowAgent({ id: agentId, ...e.workflow, type: e.subagentType ?? 'agent', label: e.description, startedAt: Date.now() })
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
    const root = buildTree(agents, usage, activity, goals, progress, todos, models, await read($, mainFinishedAtom), now, workflowAgents(), workflowLaunches(now))
    const allRows = flatten(root)
    const maxRows = Math.max(1, (e.viewport?.rows ?? 24) - ROW_CHROME)
    const { shown: rows, hidden } = visibleAgents(allRows, maxRows)
    const session = await $.session.usage({ breakdown: 'summary' })
    const quotas = mostUrgent(rateBars(session.rateLimits))
    const { breakdown } = session.context
    const limit = breakdown ? compactionLimit(breakdown) : session.context.window
    const contextPercent = limit && session.context.tokens !== undefined ? clampedPercent((session.context.tokens / limit) * 100) : clampedPercent(session.context.percent ?? 0)
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
    const SkillsSection = makeSkillsSection(ui)
    const skills = await learning.ledger(skillsOf($))
    const changeSkill = (change: Promise<void>, redrawNow = false) => {
      if (redrawNow) setTimeout(() => $.ui.invalidate('ui.render'), 0)
      void change.then(() => $.ui.invalidate('ui.render'))
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
        <Banner compact={(e.viewport?.rows ?? 24) < COMPACT_BANNER_ROWS} cwd={projectDir.cwd} />
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
        <Section title="SKILLS" icon={ICONS.steps}>
          <SkillsSection ledger={skills} isBusy={learning.isPromoting} onAccept={slug => changeSkill(learning.accept(skillsOf($), slug), true)} onReject={slug => changeSkill(learning.reject(skillsOf($), slug))} onRestore={slug => changeSkill(learning.restore(skillsOf($), slug))} onRevalidate={slug => changeSkill(learning.revalidate(skillsOf($), slug))} />
        </Section>
      </Box>
    )
  })
}
