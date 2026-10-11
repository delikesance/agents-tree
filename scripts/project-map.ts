import { spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { SUMMARY_TIMEOUT_MS, writeMap } from '../hooks/mapGenerate'
import type { MapIo } from '../hooks/mapGenerate'

const MAX_OUTPUT_BYTES = 64 * 1024 * 1024
const SUMMARY_CLI_TIMEOUT_MS = 90_000
const repo = process.argv[2] ?? process.cwd()

const exec = (argv: string[], input?: string, timeout = SUMMARY_TIMEOUT_MS) =>
  spawnSync(argv[0] ?? '', argv.slice(1), { cwd: repo, input, encoding: 'utf8', timeout, maxBuffer: MAX_OUTPUT_BYTES })

const io: MapIo = {
  summaryTimeoutMs: SUMMARY_CLI_TIMEOUT_MS,
  run: async argv => {
    const { status, stdout } = exec(argv)
    return { exitCode: status ?? 1, stdout: stdout ?? '' }
  },
  exists: async path => existsSync(join(repo, path)),
  read: path => readFile(join(repo, path), 'utf8'),
  write: async (path, text) => {
    await mkdir(dirname(join(repo, path)), { recursive: true })
    await writeFile(join(repo, path), text)
  },
  complete: async prompt => {
    const { status, stdout } = exec(['claude', '-p', '--model', 'haiku'], prompt, SUMMARY_CLI_TIMEOUT_MS)
    return status === 0 ? stdout : undefined
  },
}

const headResult = await io.run(['git', 'rev-parse', 'HEAD'])
if (headResult.exitCode !== 0) {
  console.error(`project-map: cannot resolve git HEAD in ${repo}`)
  process.exit(1)
}
await writeMap(io, headResult.stdout.trim())
