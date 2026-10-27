import { expect, test } from 'vitest'

import { matches } from '@/lib/search'

test('a query matches the start of words, in any case and without diacritics', () => {
  expect(matches('jog', ['Grčki jogurt 2 %'])).toBe(true)
  expect(matches('GRCKI JOG', ['Grčki jogurt'])).toBe(true)
  expect(matches('čevap', ['Cevapi'])).toBe(true)
  expect(matches('ogurt', ['Grčki jogurt'])).toBe(false)
  expect(matches('jogurt banana', ['Grčki jogurt'])).toBe(false)
})

test("a query's words may be in any of the texts, which may be missing", () => {
  expect(matches('skyr vanil', ['Skyr', undefined, 'Vanilla'])).toBe(true)
  expect(matches('2,8', ['Mlijeko 2,8 %'])).toBe(true)
  expect(matches('', ['Anything'])).toBe(true)
  expect(matches('  ', [])).toBe(true)
  expect(matches('a', [])).toBe(false)
})

test('a đ is typed as đ or dj, as the server finds it', () => {
  expect(matches('đuveč', ['Djuveč'])).toBe(true)
  expect(matches('djuv', ['Đuveč'])).toBe(true)
})
