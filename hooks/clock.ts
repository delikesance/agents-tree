const CLOCK_MS = 500
const STILL_NOW = 2700
const TIMER_KEY = '__agentGraphTimer'

type Timer = ReturnType<typeof setInterval>
const scope = globalThis as Record<string, unknown>
const stop = () => {
  clearInterval(scope[TIMER_KEY] as Timer | undefined)
  scope[TIMER_KEY] = undefined
}

export const mainTurn = { busy: false }
export const frameTime = () => (scope[TIMER_KEY] ? Date.now() : STILL_NOW)

export const wakeClock = (redraw: () => void, hasLiveAgent: () => Promise<boolean>) => {
  if (scope[TIMER_KEY]) return
  scope[TIMER_KEY] = setInterval(async () => {
    if (mainTurn.busy || (await hasLiveAgent().catch(() => true))) return redraw()
    stop()
    redraw()
  }, CLOCK_MS)
  redraw()
}
