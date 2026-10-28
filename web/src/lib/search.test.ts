import { expect, test } from 'vitest'

import { matches, parts } from '@/lib/search'

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

test('a đ is typed as đ, d or dj, as the server finds it', () => {
  expect(matches('đuveč', ['Djuveč'])).toBe(true)
  expect(matches('djuv', ['Đuveč'])).toBe(true)
  expect(matches('grozde', ['Bijelo grožđe'])).toBe(true)
  expect(matches('grozdje', ['Bijelo grožđe'])).toBe(true)
  expect(matches('grozd', ['Bijelo grožđe'])).toBe(true)
  expect(matches('grožđe', ['Bijelo grožđe'])).toBe(true)
  expect(matches('duvec', ['Đuveč s rižom'])).toBe(true)
  expect(matches('djuvec', ['Đuveč s rižom'])).toBe(true)
  expect(matches('duvec riz', ['Đuveč s rižom'])).toBe(true)
  // Each đ of a word the same way, as the server indexes it.
  expect(matches('djurdjevak', ['Đurđevak'])).toBe(true)
  expect(matches('durdevak', ['Đurđevak'])).toBe(true)
  expect(matches('djurdevak', ['Đurđevak'])).toBe(false)
  // A d typed is no dj.
  expect(matches('grozdi', ['Bijelo grožđe'])).toBe(false)
})

// marked is text with what a query matched in it in brackets.
function marked(query: string, text: string): string {
  return parts(query, text)
    .map((p) => (p.matched ? `[${p.text}]` : p.text))
    .join('')
}

test("a query's parts are the starts of the words it matched, as written", () => {
  expect(marked('jogu', 'Grčki jogurt 2 %')).toBe('Grčki [jogu]rt 2 %')
  expect(marked('grcki jog', 'Grčki jogurt')).toBe('[Grčki] [jog]urt')
  expect(marked('GRČ', 'grčki jogurt')).toBe('[grč]ki jogurt')
  expect(marked('2,8', 'Mlijeko 2,8 %')).toBe('Mlijeko [2],[8] %')
  // The longest of the query's words that starts a word.
  expect(marked('jo jogu', 'Jogurt')).toBe('[Jogu]rt')
  // Every word it starts.
  expect(marked('s', 'Skyr s vanilijom')).toBe('[S]kyr [s] vanilijom')
  expect(marked('ogurt', 'Grčki jogurt')).toBe('Grčki jogurt')
  expect(marked('', 'Grčki jogurt')).toBe('Grčki jogurt')
  expect(marked('jog', '')).toBe('')
})

test('a đ typed as d or dj marks the đ, and a mark written apart goes with its letter', () => {
  expect(marked('grozd', 'Bijelo grožđe')).toBe('Bijelo [grožđ]e')
  expect(marked('grozdj', 'Bijelo grožđe')).toBe('Bijelo [grožđ]e')
  expect(marked('grozde', 'Bijelo grožđe')).toBe('Bijelo [grožđe]')
  expect(marked('duv', 'Đuveč s rižom')).toBe('[Đuv]eč s rižom')
  expect(marked('dj', 'Đuveč')).toBe('[Đ]uveč')
  expect(marked('djurd', 'Đurđevak')).toBe('[Đurđ]evak')
  // A c and a caron after it, not a č.
  expect(marked('c', 'Čevapi')).toBe('[Č]evapi')
})

test('the parts are the text, each where it starts', () => {
  expect(parts('jogu', 'Grčki jogurt 2 %')).toEqual([
    { at: 0, text: 'Grčki ', matched: false },
    { at: 6, text: 'jogu', matched: true },
    { at: 10, text: 'rt 2 %', matched: false },
  ])
})
