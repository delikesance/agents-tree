import { expect, test } from 'claude-code/testing'

import { ensureProjectMap, projectMapExists } from './mapHost'

const host = (files: Record<string, string>, reply: { isAnswered: boolean; text?: string }) => {
  const runs: { argv: string[]; cwd?: string }[] = []
  return {
    runs,
    $: {
      process: {
        run: async (argv: string[], init: { cwd?: string; timeoutMs: number }) => {
          runs.push({ argv, cwd: init.cwd })
          return argv[1] === 'rev-parse' ? { exitCode: 0, stdout: 'f'.repeat(40) } : { exitCode: 0, stdout: 'src/a.ts\0' }
        },
      },
      fs: {
        read: async (path: string) => files[path] ?? Promise.reject(new Error('absent')),
        write: async (path: string, text: string) => void (files[path] = text),
        exists: async (path: string) => path in files,
      },
      model: { complete: async () => reply },
      session: { id: async () => `host-${Math.random()}` },
    },
  }
}

test('the map is written under the project directory, git runs there', async () => {
  const files: Record<string, string> = {}
  const { $, runs } = host(files, { isAnswered: true, text: 'src: code' })
  await ensureProjectMap($, '/p')
  expect(files['/p/.claude/project-map.md']).toContain('- src/ -> 1 files (.ts): code')
  expect(runs.every(({ cwd }) => cwd === '/p')).toBe(true)
})

test('an unanswered model call still writes the map', async () => {
  const files: Record<string, string> = {}
  await ensureProjectMap(host(files, { isAnswered: false }).$, '/q')
  expect(files['/q/.claude/project-map.md']).toContain('- src/ -> 1 files (.ts)\n')
})

test('the map presence is read from the project directory', async () => {
  const { $ } = host({ '/p/.claude/project-map.md': 'x' }, { isAnswered: false })
  expect(await projectMapExists($, '/p')).toBe(true)
  expect(await projectMapExists($, '/other')).toBe(false)
})
