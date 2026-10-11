import { expect, test } from 'claude-code/testing'

import { ULTRACODE_NOTE, asksUltracode, ultracodeNote } from './ultracode'

test('the keyword is found in any case, anywhere in the prompt', () => {
  expect(asksUltracode('ultracode refais le module')).toBe(true)
  expect(asksUltracode('Refais le module, UltraCode.')).toBe(true)
})

test('a longer word or a plain prompt is not an ultracode request', () => {
  expect(asksUltracode('renomme ultracoder puis lance les tests')).toBe(false)
  expect(asksUltracode('refais le module puis lance les tests')).toBe(false)
})

test('the note is given only to an ultracode request', () => {
  expect(ultracodeNote('ultracode refais le module')).toBe(ULTRACODE_NOTE)
  expect(ultracodeNote('refais le module')).toBeUndefined()
})
