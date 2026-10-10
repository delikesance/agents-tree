import { writeMap } from './mapGenerate'
import type { MapIo } from './mapGenerate'
import { MAP_FILE, mapStatus } from './mapState'

const attempted = new Set<string>()
let pendingNote: string | undefined

export const takeMapNote = () => {
  const note = pendingNote
  pendingNote = undefined
  return note
}

const noteFor = (verb: string) => `${MAP_FILE} ${verb}; subagents are pointed to it.`

export const ensureMap = async (io: MapIo, session: string) => {
  const { exitCode, stdout } = await io.run(['git', 'rev-parse', 'HEAD'])
  const head = stdout.trim()
  if (exitCode !== 0 || !head) return
  const key = `${session}:${head}`
  if (attempted.has(key)) return
  attempted.add(key)
  const status = await mapStatus(io, head)
  if (status === 'fresh' || status === 'manual') return
  await writeMap(io, head)
  pendingNote = noteFor(status === 'missing' ? 'created' : 'regenerated')
}
