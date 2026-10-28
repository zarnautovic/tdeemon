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
