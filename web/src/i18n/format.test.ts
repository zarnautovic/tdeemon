import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { daysBetween, formatGrams, parseNumber, roundedGrams, useFormat } from '@/i18n/format'
import { setLocale } from '@/i18n/locale'

afterEach(() => {
  act(() => {
    setLocale('en')
  })
})

// Sleep is hours and minutes, the minutes in two digits, everywhere: 7.999
// hours are 8 and 00, not 7 and 60.
test.each([
  [6.87, ['6', '52']],
  [7.15, ['7', '09']],
  [7.999, ['8', '00']],
  [0.5, ['0', '30']],
])('%d hours are %j', (hours, shown) => {
  const { result } = renderHook(() => useFormat())
  expect(result.current.hm(hours)).toEqual(shown)
})

test('days, a difference in percent and a list are written as the language writes them', () => {
  const { result } = renderHook(() => useFormat())
  const today = '2026-10-29'
  expect(result.current.days('2026-10-19', '2026-10-25', today)).toBe('Oct 19 – 25')
  expect(result.current.days('2025-12-29', '2026-01-04', today)).toBe('Dec 29, 2025 – Jan 4, 2026')
  expect(result.current.signedPercent(-6.7)).toBe('-7%')
  expect(result.current.signedPercent(3)).toBe('+3%')
  expect(result.current.list(['sleep', 'HRV'])).toBe('sleep and HRV')

  act(() => {
    setLocale('hr')
  })
  expect(result.current.days('2026-10-19', '2026-10-25', today)).toBe('19. – 25. lis')
  expect(result.current.signedPercent(-6.7)).toBe('−7 %')
  expect(result.current.list(['san', 'HRV'])).toBe('san i HRV')
})

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

// Grams below 10 keep a decimal, which a whole number would bend (1.5 g
// of fat is not 2); from 10 they are whole.
test.each([
  [1.5, 'en', '1.5'],
  [1.5, 'hr', '1,5'],
  [0.34, 'en', '0.3'],
  [0, 'en', '0'],
  [2, 'en', '2'],
  [9.96, 'en', '10'],
  [12.4, 'en', '12'],
  [160, 'hr', '160'],
  [1234.5, 'hr', '1.235'],
  [-1.25, 'en', '-1.3'],
  [-48.2, 'en', '-48'],
])('%d g in %s is %s', (n, locale, shown) => {
  expect(formatGrams(n, locale)).toBe(shown)
})

// Grams as a number, rounded as they are shown, to work out what is left
// of what is shown; never -0.
test.each([
  [58.7, 59],
  [1.25, 1.3],
  [9.96, 10],
  [160, 160],
  [-0.04, 0],
])('%d g are %d as shown', (n, shown) => {
  expect(roundedGrams(n)).toBe(shown)
})

test.each([
  ['2026-10-01', '2026-10-01', 0],
  ['2026-10-01', '2026-10-27', 26],
  ['2026-10-27', '2026-10-01', -26],
  // Across the end of summer time, a day is a day.
  ['2026-10-24', '2026-10-26', 2],
  ['2026-12-31', '2027-01-01', 1],
])('from %s to %s is %d days', (from, to, n) => {
  expect(daysBetween(from, to)).toBe(n)
})
