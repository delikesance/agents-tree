import type { SkillDeps } from './skillHost'
import { groupSignal, isInactive, readyToPromote, reject, remove, removeInactive, restore } from './skillLedger'
import { recordUse, revalidate } from './skillLifecycle'
import { closeSession, flagStale, isLive } from './skillMaintenance'
import { promotionPatch } from './skillPromotion'
import { summarizeCandidate } from './skillSummary'
import { adoptVerdicts, discoverVerdicts, verdictLines } from './skillDiscover'
import { createSignalDetector } from './skillSignals'
import { SKILLS_STORE_KEY, SYNTHESIS_ENV, type SkillLedger, type SkillRecord, type SkillSignal } from './skillTypes'
import { headCommit, removeSkill } from './skillWriter'

const MAX_PROMOTE_ATTEMPTS = 3
const SESSION_STORE_KEY = `${SKILLS_STORE_KEY}.session`
const FILE_ARGS = ['file_path', 'path', 'notebook_path']

const filesOf = (call: Record<string, unknown>) => FILE_ARGS.map(key => call[key]).filter((value): value is string => typeof value === 'string' && value !== '')

const dropSkillFiles = async (deps: SkillDeps, records: SkillRecord[]) => {
  const { cwd } = deps
  if (cwd) await Promise.all(records.filter(isLive).map(record => removeSkill(deps.fs, cwd, record.slug)))
}

export function createSkillLearning() {
  const detector = createSignalDetector()
  const attempts = new Map<string, number>()
  const promoting = new Set<string>()
  const summarizing = new Set<string>()
  const summaryAttempts = new Map<string, number>()
  const session = { id: '', turns: 0, corrected: false, used: [] as string[], awaiting: [] as string[] }
  let cache: SkillLedger | undefined
  let queue: Promise<unknown> = Promise.resolve()

  const load = async (deps: SkillDeps) => (cache ??= ((await deps.store.get(SKILLS_STORE_KEY)) as SkillLedger | undefined) ?? {})

  const mutate = (deps: SkillDeps, change: (ledger: SkillLedger) => SkillLedger | Promise<SkillLedger>) => {
    if (typeof process !== 'undefined' && process.env?.[SYNTHESIS_ENV]) return queue
    queue = queue
      .then(async () => {
        const before = await load(deps)
        const after = await change(before)
        if (after === before) return
        cache = after
        await deps.store.set(SKILLS_STORE_KEY, after)
      })
      .catch(() => undefined)
    return queue as Promise<void>
  }

  const settleUses = (deps: SkillDeps, ledger: SkillLedger, clean: boolean) =>
    session.awaiting.splice(0).reduce((current, slug) => recordUse(current, slug, deps.now(), clean, session.id), ledger)

  const learn = (deps: SkillDeps, ledger: SkillLedger, signals: SkillSignal[]) =>
    startPromotions(deps, signals.reduce((current, signal) => groupSignal(current, signal).ledger, ledger))

  const applyPromotion = async (deps: SkillDeps, ledger: SkillLedger, slug: string, patch: NonNullable<Awaited<ReturnType<typeof promotionPatch>>>) => {
    const record = ledger[slug]
    if (record?.status === 'candidate') return { ...ledger, [slug]: { ...record, ...patch } }
    if (deps.cwd) await removeSkill(deps.fs, deps.cwd, slug)
    return ledger
  }

  const promote = async (deps: SkillDeps, cwd: string, slug: string) => {
    const record = (await load(deps))[slug]
    const patch = record && (await promotionPatch(deps, cwd, record).catch(() => undefined))
    promoting.delete(slug)
    if (patch) await mutate(deps, ledger => applyPromotion(deps, ledger, slug, patch))
  }

  const withSummary = (ledger: SkillLedger, slug: string, summary: string): SkillLedger =>
    ledger[slug] ? { ...ledger, [slug]: { ...ledger[slug], summary } } : ledger

  const summarize = async (deps: SkillDeps, record: SkillRecord) => {
    const summary = await summarizeCandidate(record, deps.fs)
    summarizing.delete(record.slug)
    if (summary) await mutate(deps, ledger => withSummary(ledger, record.slug, summary))
  }

  const startSummaries = (deps: SkillDeps, ledger: SkillLedger) => {
    for (const record of Object.values(ledger)) {
      const { slug } = record
      const exhausted = (summaryAttempts.get(slug) ?? 0) >= MAX_PROMOTE_ATTEMPTS
      if (record.status !== 'candidate' || record.summary || exhausted || summarizing.has(slug)) continue
      summarizing.add(slug)
      summaryAttempts.set(slug, (summaryAttempts.get(slug) ?? 0) + 1)
      void summarize(deps, record)
    }
    return ledger
  }

  const startPromotions = (deps: SkillDeps, ledger: SkillLedger) => {
    const { cwd } = deps
    const liveCount = Object.values(ledger).filter(isLive).length
    for (const record of Object.values(ledger)) {
      const { slug } = record
      const exhausted = (attempts.get(slug) ?? 0) >= MAX_PROMOTE_ATTEMPTS
      if (!cwd || exhausted || promoting.has(slug) || !readyToPromote(record, liveCount)) continue
      promoting.add(slug)
      attempts.set(slug, (attempts.get(slug) ?? 0) + 1)
      void promote(deps, cwd, slug)
    }
    return startSummaries(deps, ledger)
  }

  const enterSession = async (deps: SkillDeps, ledger: SkillLedger) => {
    const id = await deps.sessionId()
    if (id === session.id) return ledger
    const previous = session.id || ((await deps.store.get(SESSION_STORE_KEY)) as string | undefined)
    session.awaiting.push(...session.used.splice(0))
    const settled = settleUses(deps, ledger, true)
    Object.assign(session, { id, turns: 0, corrected: false })
    await deps.store.set(SESSION_STORE_KEY, id)
    if (!previous || previous === id) return startPromotions(deps, await flagStale(deps, settled))
    detector.endSession(previous)
    return startPromotions(deps, await flagStale(deps, await closeSession(deps, settled, previous)))
  }

  const run = (deps: SkillDeps, task: (ledger: SkillLedger) => SkillLedger | Promise<SkillLedger>) =>
    mutate(deps, async ledger => task(await enterSession(deps, ledger)))

  const resetIdle = (ledger: SkillLedger, slug: string): SkillLedger => (ledger[slug] ? { ...ledger, [slug]: { ...ledger[slug], idleSessions: 0 } } : ledger)

  return {
    ledger: (deps: SkillDeps) => load(deps),
    isPromoting: (slug: string) => promoting.has(slug),
    onSessionStart: (deps: SkillDeps) => run(deps, ledger => ledger),
    onTool: (deps: SkillDeps, call: Record<string, unknown> & { tool: string }, failed: boolean) =>
      run(deps, ledger => learn(deps, ledger, detector.push({ type: 'tool', sessionId: session.id, at: deps.now(), tool: call.tool, files: filesOf(call), failed }))),
    onPrompt: (deps: SkillDeps, text: string) =>
      run(deps, ledger => {
        const signals = detector.push({ type: 'prompt', sessionId: session.id, at: deps.now(), text })
        const corrected = signals.some(signal => signal.kind === 'correction')
        session.corrected ||= corrected
        return learn(deps, settleUses(deps, ledger, !corrected), signals)
      }),
    onTurnComplete: (deps: SkillDeps) =>
      run(deps, ledger => {
        session.turns += 1
        const signals = detector.push({ type: 'turn-end', sessionId: session.id, at: deps.now(), turns: session.turns, corrected: session.corrected })
        session.corrected = false
        session.awaiting.push(...session.used.splice(0))
        return learn(deps, ledger, signals)
      }),
    onSkillPrompt: (deps: SkillDeps, slug: string) =>
      run(deps, ledger => {
        if (ledger[slug]) session.used.push(slug)
        return ledger
      }),
    discover: async (deps: SkillDeps, apply: boolean) => {
      const { cwd } = deps
      if (!cwd) return 'Aucun projet ouvert.'
      const verdicts = await discoverVerdicts(deps, cwd, await load(deps))
      if (apply) void mutate(deps, ledger => adoptVerdicts(deps, cwd, ledger, verdicts))
      if (!verdicts.length) return 'Aucun candidat dans CLAUDE.md / README.md.'
      return apply ? `${verdictLines(verdicts)}\nRédaction des skills ✔ en arrière-plan.` : verdictLines(verdicts)
    },
    reject: (deps: SkillDeps, slug: string) =>
      mutate(deps, async ledger => {
        if (ledger[slug] && isLive(ledger[slug]) && deps.cwd) await removeSkill(deps.fs, deps.cwd, slug)
        return reject(ledger, slug)
      }),
    accept: async (deps: SkillDeps, slug: string) => {
      const { cwd } = deps
      const record = (await load(deps))[slug]
      if (!cwd || record?.status !== 'candidate' || promoting.has(slug)) return
      promoting.add(slug)
      await promote(deps, cwd, slug)
    },
    restore: (deps: SkillDeps, slug: string) => mutate(deps, ledger => startPromotions(deps, restore(resetIdle(ledger, slug), slug))),
    dismiss: (deps: SkillDeps, slug: string) =>
      mutate(deps, async ledger => {
        const record = ledger[slug]
        if (record) await dropSkillFiles(deps, [record])
        return remove(ledger, slug)
      }),
    dismissInactive: (deps: SkillDeps) =>
      mutate(deps, async ledger => {
        await dropSkillFiles(deps, Object.values(ledger).filter(isInactive))
        return removeInactive(ledger)
      }),
    revalidate: (deps: SkillDeps, slug: string) => mutate(deps, async ledger => revalidate(ledger, slug, await headCommit(deps.fs))),
  }
}
