export const CLOCK_MS = 500
export const SPINNER_MS = CLOCK_MS
const STILL_NOW = 2700
const TIMER_KEY = '__agentGraphTimer'

type Timer = ReturnType<typeof setTimeout>
const scope = globalThis as Record<string, unknown>
const stop = (timer: Timer) => {
  if (scope[TIMER_KEY] !== timer) return
  clearTimeout(timer)
  scope[TIMER_KEY] = undefined
}

export const mainTurn = { busy: false }
export const frameTime = () => (scope[TIMER_KEY] ? Date.now() : STILL_NOW)

const schedule = (redraw: () => void, hasLiveAgent: () => Promise<boolean>) => {
  const timer: Timer = setTimeout(async () => {
    const alive = mainTurn.busy || (await hasLiveAgent().catch(() => false))
    if (scope[TIMER_KEY] !== timer) return
    if (alive) schedule(redraw, hasLiveAgent)
    else stop(timer)
    redraw()
  }, CLOCK_MS)
  scope[TIMER_KEY] = timer
}

export const wakeClock = (redraw: () => void, hasLiveAgent: () => Promise<boolean>) => {
  if (scope[TIMER_KEY]) return
  schedule(redraw, hasLiveAgent)
  redraw()
}
