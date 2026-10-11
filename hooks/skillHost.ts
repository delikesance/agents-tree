import type { SkillFs } from './skillTypes'

const RUN_TIMEOUT_MS = 90_000

export type SkillDeps = {
  store: { get: (key: string) => Promise<unknown>; set: (key: string, value: unknown) => Promise<void> }
  fs: SkillFs
  cwd?: string
  now: () => number
  sessionId: () => Promise<string>
}

type SkillHostSource = {
  store: SkillDeps['store']
  fs: { read: (path: string) => Promise<string>; write: (path: string, text: string) => Promise<void>; exists: (path: string) => Promise<boolean> }
  process: { run: (argv: string[], init: { cwd?: string; timeoutMs: number }) => Promise<{ exitCode: number; stdout: string }> }
  session: { id: () => Promise<string> }
}

const directoryOf = (path: string) => path.slice(0, path.lastIndexOf('/'))

const skillFsOf = ($: SkillHostSource, cwd?: string): SkillFs => {
  const run = async (command: string[]) => {
    const { exitCode, stdout } = await $.process.run(command, { cwd, timeoutMs: RUN_TIMEOUT_MS })
    if (exitCode !== 0) throw new Error(`${command[0]} exited with ${exitCode}`)
    return stdout
  }
  return {
    read: async path => ((await $.fs.exists(path)) ? $.fs.read(path) : undefined),
    write: (path, text) => $.fs.write(path, text),
    remove: async path => {
      await run(['rm', '-f', path])
      await run(['rmdir', directoryOf(path)]).catch(() => undefined)
    },
    run,
  }
}

export const skillDepsOf = ($: SkillHostSource, cwd?: string): SkillDeps => ({
  store: { get: key => $.store.get(key), set: (key, value) => $.store.set(key, value) },
  fs: skillFsOf($, cwd),
  cwd,
  now: Date.now,
  sessionId: () => $.session.id(),
})
