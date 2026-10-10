import { withTimeout } from './brief'
import { MAP_FILE } from './mapState'
import type { MapReader } from './mapState'
import { SUMMARY_PROMPT, buildMap, parseSummaries, summaryInput, topDirectories } from './mapSkeleton'
import { detectTestCommand } from './testCommand'

const SUMMARY_TIMEOUT_MS = 20_000

export type MapIo = MapReader & {
  write: (path: string, text: string) => Promise<void>
  complete: (prompt: string) => Promise<string | undefined>
}

const trackedFiles = async (run: Run) => {
  const { exitCode, stdout } = await run(['git', 'ls-files', '-z'])
  if (exitCode !== 0) throw new Error('git ls-files failed')
  return stdout.split('\0').filter(Boolean)
}

const summariesOf = async (complete: MapIo['complete'], files: string[]) => {
  const dirs = topDirectories(files)
  const text = await withTimeout(complete(SUMMARY_PROMPT + summaryInput(dirs)), SUMMARY_TIMEOUT_MS).catch(() => undefined)
  return parseSummaries(text ?? '', dirs)
}

export const writeMap = async ({ run, read, write, complete }: MapIo, head: string) => {
  const files = await trackedFiles(run)
  const [testCommand, summaries] = await Promise.all([detectTestCommand(read), summariesOf(complete, files)])
  await write(MAP_FILE, buildMap({ head, files, testCommand, summaries }))
}
