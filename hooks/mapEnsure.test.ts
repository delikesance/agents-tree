import { expect, test } from 'claude-code/testing'

import { ensureMap, takeMapNote } from './mapEnsure'
import type { MapIo } from './mapGenerate'
import { MAP_FILE } from './mapState'

const HEAD = 'd'.repeat(40)
const OLD = 'e'.repeat(40)

type Repo = { head?: string; files?: string[]; changed?: string; map?: string; summary?: string | Error }

const setup = ({ head = HEAD, files = ['src/main.ts', 'src/a.ts'], changed = '', map, summary = 'src: code' }: Repo) => {
  const written: string[] = []
  let completions = 0
  const io: MapIo = {
    run: async argv => {
      if (argv[1] === 'rev-parse') return head ? { exitCode: 0, stdout: `${head}\n` } : { exitCode: 128, stdout: '' }
      if (argv[1] === 'ls-files') return { exitCode: 0, stdout: files.join('\0') + '\0' }
      return { exitCode: 0, stdout: changed }
    },
    read: async path => (path === MAP_FILE && map !== undefined ? map : Promise.reject(new Error('absent'))),
    write: async (_path, text) => void written.push(text),
    complete: async () => {
      completions += 1
      if (summary instanceof Error) throw summary
      return summary
    },
  }
  return { io, written, completions: () => completions }
}

test('a missing map is generated and noted once', async () => {
  const { io, written } = setup({})
  await ensureMap(io, 'missing')
  expect(written).toHaveLength(1)
  expect(written[0]?.split('\n')[0]).toBe(`<!-- map: ${HEAD} -->`)
  expect(written[0]).toContain('- src/ -> 2 files (.ts): code')
  expect(takeMapNote()).toContain('created')
  expect(takeMapNote()).toBeUndefined()
})

test('a map written at HEAD is left alone', async () => {
  const { io, written, completions } = setup({ map: `<!-- map: ${HEAD} -->\n` })
  await ensureMap(io, 'fresh')
  expect(written).toHaveLength(0)
  expect(completions()).toBe(0)
  expect(takeMapNote()).toBeUndefined()
})

test('HEAD moved without a tracked file added, deleted or renamed leaves the map alone', async () => {
  const { io, written } = setup({ map: `<!-- map: ${OLD} -->\n`, changed: '' })
  await ensureMap(io, 'moved')
  expect(written).toHaveLength(0)
})

test('a structural change regenerates the map', async () => {
  const { io, written } = setup({ map: `<!-- map: ${OLD} -->\n`, changed: 'A\tsrc/a.ts\n' })
  await ensureMap(io, 'stale')
  expect(written).toHaveLength(1)
  expect(takeMapNote()).toContain('regenerated')
})

test('a map without commit line is never overwritten', async () => {
  const { io, written } = setup({ map: '# my own map\n', changed: 'A\tx\n' })
  await ensureMap(io, 'manual')
  expect(written).toHaveLength(0)
})

test('a model failure writes the map without summaries', async () => {
  const { io, written } = setup({ summary: new Error('down') })
  await ensureMap(io, 'failing')
  expect(written).toHaveLength(1)
  expect(written[0]).toContain('- src/ -> 2 files (.ts)\n')
  takeMapNote()
})

test('outside a git repository nothing happens', async () => {
  const { io, written, completions } = setup({ head: '' })
  await ensureMap(io, 'norepo')
  expect(written).toHaveLength(0)
  expect(completions()).toBe(0)
})

test('one attempt per session and HEAD, kept when generation fails', async () => {
  const { io, written } = setup({})
  const failing: MapIo = { ...io, write: async () => Promise.reject(new Error('disk')) }
  await ensureMap(failing, 'once').catch(() => undefined)
  await ensureMap(io, 'once')
  expect(written).toHaveLength(0)
  await ensureMap(io, 'other')
  expect(written).toHaveLength(1)
  takeMapNote()
})
