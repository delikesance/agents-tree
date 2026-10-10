// The runtime provides these globals; the plugin tsconfig has neither a DOM nor a Node lib.
type TimerHandle = number

declare var setTimeout: (handler: (...args: never[]) => void, ms?: number) => TimerHandle
declare var clearTimeout: (timer: TimerHandle) => void
declare var process: {
  on: (event: 'unhandledRejection', listener: (reason: unknown) => void) => void
  off: (event: 'unhandledRejection', listener: (reason: unknown) => void) => void
}
