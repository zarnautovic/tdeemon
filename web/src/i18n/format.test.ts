import { expect, test } from 'vitest'

import { parseNumber } from '@/i18n/format'

test.each([
  ['72.5', 'en', 72.5],
  ['72,5', 'en', 72.5],
  ['72.5', 'hr', 72.5],
  ['72,5', 'hr', 72.5],
  ['8,042', 'en', 8042],
  ['8.042', 'hr', 8042],
  ['1,234,567', 'en', 1_234_567],
  ['8 042', 'hr', 8042],
  [' 70 ', 'en', 70],
  ['0', 'en', 0],
])('%s in %s is %d', (typed, locale, n) => {
  expect(parseNumber(typed, locale)).toBe(n)
})

test.each([
  ['', 'en'],
  ['abc', 'en'],
  ['72.5 kg', 'en'],
  ['-1', 'en'],
  ['1.2.3', 'en'],
  ['1,2,3', 'hr'],
  ['72.', 'en'],
  ['.5', 'en'],
  ['1e3', 'en'],
])('%j in %s is no number', (typed, locale) => {
  expect(parseNumber(typed, locale)).toBeUndefined()
})
