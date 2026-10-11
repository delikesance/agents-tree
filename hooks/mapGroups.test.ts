import { expect, test } from 'claude-code/testing'

import { MAX_GROUPS, groupFiles } from './mapGroups'

const names = (files: string[]) => groupFiles(files).map(group => group.name)
const numbered = (dir: string, prefix: string, count: number) => Array.from({ length: count }, (_, i) => `${dir}/${prefix}${String.fromCharCode(65 + i)}.ts`)

test('workspace members get one group each', () => {
  const files = ['crates/a/src/main.rs', 'crates/b/src/lib.rs', 'crates/c/Cargo.toml', 'Cargo.toml']
  expect(names(files)).toEqual(['./*', 'crates/a/', 'crates/b/', 'crates/c/'])
})

test('a large nested directory is split deeper', () => {
  const files = [...numbered('web/src/chat', 'msg', 16).map(file => file.replace('.ts', '.tsx')), ...numbered('web/src/ui', 'u', 3)]
  expect(names(files)).toContain('web/src/chat/msg*')
  expect(names(files)).toContain('web/src/ui/')
})

test('a large flat directory splits by filename prefix and singletons fall into other', () => {
  const files = [...numbered('hooks', 'mapPart', 8), ...numbered('hooks', 'flowStep', 7), 'hooks/zeta.ts', 'hooks/README.md']
  expect(names(files)).toEqual(['hooks/map*', 'hooks/flow*', 'hooks/other'])
})

test('test files are counted apart from code files', () => {
  const [group] = groupFiles(['src/a.ts', 'src/b.ts', 'src/a.test.ts', 'src/c.spec.ts', 'src/d_test.go'])
  expect(group?.files).toEqual(['src/a.ts', 'src/b.ts'])
  expect(group?.tests).toHaveLength(3)
})

test('too many groups are merged, never dropped', () => {
  const files = Array.from({ length: 200 }, (_, i) => `dir${i}/a.ts`)
  const groups = groupFiles(files)
  expect(groups.length).toBeLessThanOrEqual(MAX_GROUPS)
  expect(groups.flatMap(group => group.files)).toHaveLength(200)
  expect(groups.reduce((sum, group) => sum + group.merged, 0)).toBe(200 - groups.length)
})
