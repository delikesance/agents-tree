import { ensureMap } from './mapEnsure'
import type { MapIo } from './mapGenerate'
import { MAP_FILE } from './mapState'
import { MAP_SUMMARY_MAX_TOKENS } from './mapSkeleton'

const GIT_TIMEOUT_MS = 10_000

type MapHost = {
  process: { run: (argv: string[], init: { cwd?: string; timeoutMs: number }) => Promise<{ exitCode: number; stdout: string }> }
  fs: { read: (path: string) => Promise<string>; write: (path: string, text: string) => Promise<void>; exists: (path: string) => Promise<boolean> }
  model: { complete: (request: { model: string; prompt: string; effort: 'low'; maxTokens: number }) => Promise<{ isAnswered: boolean; text?: string }> }
  session: { id: () => Promise<string> }
}

const inProject = (cwd: string | undefined, path: string) => (cwd ? `${cwd}/${path}` : path)

const mapIo = ($: MapHost, cwd?: string): MapIo => ({
  run: argv => $.process.run(argv, { cwd, timeoutMs: GIT_TIMEOUT_MS }),
  read: path => $.fs.read(inProject(cwd, path)),
  write: (path, text) => $.fs.write(inProject(cwd, path), text),
  complete: async prompt => {
    const reply = await $.model.complete({ model: 'haiku', prompt, effort: 'low', maxTokens: MAP_SUMMARY_MAX_TOKENS })
    return reply.isAnswered ? reply.text : undefined
  },
})

export const ensureProjectMap = async ($: MapHost, cwd?: string) => ensureMap(mapIo($, cwd), await $.session.id())

export const projectMapExists = ($: Pick<MapHost, 'fs'>, cwd?: string) => $.fs.exists(inProject(cwd, MAP_FILE)).catch(() => false)
