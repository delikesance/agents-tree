import { expect, test } from 'claude-code/testing'

import { extractSymbols, readSymbols, symbolEntry } from './mapSymbols'

test('TypeScript exports are extracted, plain declarations are not', () => {
  const text = 'export const A = 1\nexport async function b() {}\nexport type C = {}\nexport interface D {}\nconst hidden = 1\n  export const indented = 2\n'
  expect(extractSymbols('x.tsx', text)).toEqual(['A', 'b', 'C', 'D'])
})

test('Rust top-level pub items are extracted, methods are not', () => {
  const text = 'pub fn run() {}\npub struct Cli;\npub(crate) enum Mode {}\npub trait Tool {}\npub mod util;\nfn private() {}\nimpl Cli {\n    pub fn method() {}\n}\n'
  expect(extractSymbols('src/lib.rs', text)).toEqual(['run', 'Cli', 'Mode', 'Tool', 'util'])
})

test('Python top-level defs and classes are extracted, private ones are not', () => {
  expect(extractSymbols('a.py', 'def run():\n  pass\nclass Cli:\n  def method(self): pass\ndef _hidden(): pass\n')).toEqual(['run', 'Cli'])
})

test('only source files are read and files without symbols are left out', async () => {
  const reads: string[] = []
  const read = async (path: string) => (reads.push(path), path === 'a.ts' ? 'export const A = 1' : 'nothing')
  const symbols = await readSymbols(read, ['a.ts', 'b.ts', 'c.json'])
  expect(reads).toEqual(['a.ts', 'b.ts'])
  expect([...symbols.keys()]).toEqual(['a.ts'])
  expect(symbolEntry('src/a.ts', new Map([['src/a.ts', ['A', 'B']]]))).toBe('a.ts(A, B)')
})
