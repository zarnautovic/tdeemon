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
  ['1,834.5', 'en', 1834.5],
  ['1.834,27', 'hr', 1834.27],
  // A 0 starts no thousands: these are decimals.
  ['0.125', 'hr', 0.125],
  ['0,125', 'en', 0.125],
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
  ['1.834,5', 'en'],
  ['1,834.5', 'hr'],
])('%j in %s is no number', (typed, locale) => {
  expect(parseNumber(typed, locale)).toBeUndefined()
})
