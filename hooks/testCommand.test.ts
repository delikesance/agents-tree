import { expect, test } from 'claude-code/testing'

import { detectTestCommand } from './testCommand'

const files = (contents: Record<string, string>) => (path: string) => (path in contents ? Promise.resolve(contents[path] as string) : Promise.reject(new Error('absent')))

test('the claudework config wins', async () => {
  const read = files({ '.claudework/config.json': '{"test_command":"make check"}', 'package.json': '{"scripts":{"test":"x"}}' })
  expect(await detectTestCommand(read)).toBe('make check')
})

test('manifests give a default command', async () => {
  expect(await detectTestCommand(files({ 'package.json': '{"scripts":{"test":"x"}}' }))).toBe('npm test')
  expect(await detectTestCommand(files({ 'Cargo.toml': '' }))).toBe('cargo test')
  expect(await detectTestCommand(files({ 'pyproject.toml': '' }))).toBe('python3 -m pytest -q')
})

test('no source gives no command', async () => {
  expect(await detectTestCommand(files({ 'package.json': '{}', '.claudework/config.json': 'not json' }))).toBeUndefined()
})
