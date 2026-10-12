import type { TaskExec, TaskIo } from './taskStore'

export type TaskFs = {
  exists(path: string): Promise<boolean>
  read(path: string): Promise<string>
  write(path: string, text: string): Promise<void>
}

export type RunProcess = (argv: string[], cwd?: string) => Promise<{ exitCode: number; stdout: string; stderr: string }>

export type TaskRuntime = {
  io: TaskIo
  exec: TaskExec
  setStore(key: string, value: unknown): Promise<void>
}

export const hostTaskIo = (fs: TaskFs, run: RunProcess): TaskIo => ({
  read: async path => ((await fs.exists(path)) ? fs.read(path) : undefined),
  write: (path, content) => fs.write(path, content),
  rename: async (from, to) => {
    const { exitCode, stderr } = await run(['mv', from, to])
    if (exitCode !== 0) throw new Error(stderr)
  },
})

export const hostTaskExec =
  (run: RunProcess): TaskExec =>
  async (cmd, cwd) => {
    try {
      const { exitCode, stdout, stderr } = await run(['bash', '-lc', cmd], cwd)
      return { code: exitCode, output: stdout + stderr }
    } catch (error) {
      return { code: 1, output: error instanceof Error ? error.message : String(error) }
    }
  }
