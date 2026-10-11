import { withTimeout } from './brief'
import { groupFiles } from './mapGroups'
import type { Group } from './mapGroups'
import { manualBlock } from './mapManual'
import { MAP_FILE } from './mapState'
import type { MapReader, Run } from './mapState'
import { buildMap } from './mapSkeleton'
import { SUMMARY_PROMPT, parseSummaries, summaryInput } from './mapSummaries'
import { readSymbols } from './mapSymbols'
import type { Symbols } from './mapSymbols'
import { detectTestCommand } from './testCommand'

export const SUMMARY_TIMEOUT_MS = 20_000

export type MapIo = MapReader & {
  write: (path: string, text: string) => Promise<void>
  complete: (prompt: string) => Promise<string | undefined>
  summaryTimeoutMs?: number
}

const trackedFiles = async (run: Run) => {
  const { exitCode, stdout } = await run(['git', 'ls-files', '-z'])
  if (exitCode !== 0) throw new Error('git ls-files failed')
  return stdout.split('\0').filter(Boolean)
}

const summariesOf = async ({ complete, summaryTimeoutMs = SUMMARY_TIMEOUT_MS }: MapIo, groups: Group[], symbols: Symbols) => {
  const text = await withTimeout(complete(SUMMARY_PROMPT + summaryInput(groups, symbols)), summaryTimeoutMs).catch(() => undefined)
  return parseSummaries(text ?? '', groups)
}

export const writeMap = async (io: MapIo, head: string) => {
  const { run, read, write } = io
  const files = await trackedFiles(run)
  const groups = groupFiles(files)
  const [testCommand, symbols, existing] = await Promise.all([
    detectTestCommand(read),
    readSymbols(read, groups.flatMap(group => group.files)),
    read(MAP_FILE).catch(() => undefined),
  ])
  const summaries = await summariesOf(io, groups, symbols)
  await write(MAP_FILE, buildMap({ head, files, groups, testCommand, summaries, symbols, manual: manualBlock(existing) }))
}
