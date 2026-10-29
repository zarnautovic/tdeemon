import { expect, test } from 'vitest'

import { en } from '@/i18n/en'
import { hr } from '@/i18n/hr'

// textsOf is every text of messages by its key, a text with values
// given 1 for each: what a percentage in it looks like.
function textsOf(messages: object, at = ''): [string, string][] {
  return Object.entries(messages).flatMap(([key, m]: [string, unknown]): [string, string][] => {
    const path = at === '' ? key : `${at}.${key}`
    if (typeof m === 'string') {
      return [[path, m]]
    }
    if (typeof m === 'function') {
      const text: unknown = Reflect.apply(
        m,
        undefined,
        Array.from({ length: m.length }, () => '1'),
      )
      return [[path, String(text)]]
    }
    return typeof m === 'object' && m !== null ? textsOf(m, path) : []
  })
}

// English writes a percentage as Intl does, the sign right after the
// number (17.3%), so the same number reads the same on every screen.
test('no English message puts a space before %', () => {
  const spaced = textsOf(en).filter(([, text]) => /\s%/.test(text))
  expect(spaced).toEqual([])
})

// Croatian puts a space between the number and the sign (17,3 %).
test('no Croatian message puts a number right before %', () => {
  const tight = textsOf(hr).filter(([, text]) => /\d%/.test(text))
  expect(tight).toEqual([])
  expect(textsOf(hr).filter(([, text]) => /\d %/.test(text)).length).toBeGreaterThan(0)
})

// astray are the keys of the texts that say one of words.
function astray(texts: [string, string][], words: RegExp): string[] {
  return texts.filter(([, text]) => words.test(text)).map(([key]) => key)
}

// The glossary (docs/conventions.md, "Words"): one name for the AI coach
// and one for the weight, in each language.
test("the messages call the coach the AI coach, and the weight the weight's word", () => {
  expect(astray(textsOf(en), /\b(a coach|the coach|models?|AI clients?)\b/i)).toEqual([])
  expect(astray(textsOf(hr), /\b(coach\w*|model\w*|AI klijent\w*|težin\w*)/i)).toEqual([])
  // A serving is a "mjera" in Croatian.
  expect(astray(textsOf(hr), /\bserving\w*/i)).toEqual([])
})

// What a device or an AI coach signs in with is its access key, the
// API's token, never named so.
test('the messages call a token an access key', () => {
  expect(astray(textsOf(en), /\btokens?\b/i)).toEqual([])
  expect(astray(textsOf(hr), /\btoken\w*/i)).toEqual([])
})

// long are the keys of the texts with a sentence of more than 20 words.
function long(texts: [string, string][]): string[] {
  return texts
    .filter(([, text]) =>
      text.split(/(?<=[.?!])\s+/).some((sentence) => sentence.split(/\s+/).length > 20),
    )
    .map(([key]) => key)
}

// The voice (docs/conventions.md, "Words"): a sentence of 20 words at
// most, but the list of Health Auto Export's metrics to tick, which are
// its names.
test('a sentence of a message is 20 words at most', () => {
  expect(long(textsOf(en))).toEqual(['connect.hae.metrics'])
  expect(long(textsOf(hr))).toEqual(['connect.hae.metrics'])
})

// counts are how many words each text has.
function counts(texts: [string, string][]): number[] {
  return texts.map(([, text]) => text.split(/\s+/).length)
}

// What is wrong with an amount or an estimate is said at its field, in
// 15 words at most.
test('what is wrong with an amount is said in 15 words at most', () => {
  expect(
    Math.max(...counts(textsOf(en.log.wrong)), ...counts(textsOf(hr.log.wrong))),
  ).toBeLessThanOrEqual(15)
})
