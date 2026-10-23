import { afterEach, expect, test, vi } from 'vitest'

import { detectLocale, getLocale, setLocale } from '@/i18n/locale'

afterEach(() => {
  vi.restoreAllMocks()
  setLocale('en')
  localStorage.clear()
})

test('the language chosen last in this browser comes first', () => {
  expect(detectLocale('hr', ['en-US'])).toBe('hr')
})

test("else the first of the browser's languages the app speaks, by its language", () => {
  expect(detectLocale(null, ['de-AT', 'hr-HR', 'en'])).toBe('hr')
  expect(detectLocale(null, ['HR'])).toBe('hr')
  expect(detectLocale('xx', ['en-GB', 'hr'])).toBe('en')
})

test('else English', () => {
  expect(detectLocale(null, ['de-DE', 'fr'])).toBe('en')
  expect(detectLocale(null, [])).toBe('en')
})

test('a language set is kept for the next visit', () => {
  setLocale('hr')

  expect(getLocale()).toBe('hr')
  expect(localStorage.getItem('tdeemon.locale')).toBe('hr')
  expect(detectLocale()).toBe('hr')
})

test('a language is set for this visit when the browser keeps nothing', () => {
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('The quota has been exceeded.', 'QuotaExceededError')
  })

  setLocale('hr')

  expect(getLocale()).toBe('hr')
  expect(localStorage.getItem('tdeemon.locale')).toBeNull()
})
