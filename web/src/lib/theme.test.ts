import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { applyTheme, getTheme, readTheme, setTheme } from '@/lib/theme'

// The page's colours for the browser's bar, as index.html has them.
beforeEach(() => {
  document.head.innerHTML = `
    <meta name="theme-color" data-scheme="light" media="(prefers-color-scheme: light)" content="#F5F6F8" />
    <meta name="theme-color" data-scheme="dark" media="(prefers-color-scheme: dark)" content="#0A0E15" />`
})

afterEach(() => {
  vi.restoreAllMocks()
  setTheme('system')
  localStorage.clear()
  document.head.innerHTML = ''
})

const root = document.documentElement

function barMedia() {
  return [...document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')].map((meta) => [
    meta.dataset.scheme,
    meta.media,
  ])
}

test("the theme kept in this browser is read back, else it is the system's", () => {
  expect(readTheme('dark')).toBe('dark')
  expect(readTheme('light')).toBe('light')
  expect(readTheme('system')).toBe('system')
  expect(readTheme(null)).toBe('system')
  expect(readTheme('sepia')).toBe('system')
})

test('a theme chosen marks the page and is kept for the next visit', () => {
  setTheme('dark')

  expect(getTheme()).toBe('dark')
  expect(root.classList.contains('dark')).toBe(true)
  expect(root.classList.contains('light')).toBe(false)
  expect(localStorage.getItem('tdeemon.theme')).toBe('dark')
  expect(readTheme()).toBe('dark')

  setTheme('light')

  expect(root.classList.contains('light')).toBe(true)
  expect(root.classList.contains('dark')).toBe(false)
  expect(readTheme()).toBe('light')
})

test("the system's theme leaves the page to the system's colours", () => {
  setTheme('dark')
  setTheme('system')

  expect(root.classList.contains('dark')).toBe(false)
  expect(root.classList.contains('light')).toBe(false)
  expect(readTheme()).toBe('system')
})

test("the browser's bar takes the colour of the theme chosen, or the system's", () => {
  setTheme('dark')
  expect(barMedia()).toEqual([
    ['light', 'not all'],
    ['dark', 'all'],
  ])

  setTheme('light')
  expect(barMedia()).toEqual([
    ['light', 'all'],
    ['dark', 'not all'],
  ])

  setTheme('system')
  expect(barMedia()).toEqual([
    ['light', '(prefers-color-scheme: light)'],
    ['dark', '(prefers-color-scheme: dark)'],
  ])
})

test('the theme kept is put on the page when the app starts', () => {
  localStorage.setItem('tdeemon.theme', 'dark')

  applyTheme(readTheme())

  expect(root.classList.contains('dark')).toBe(true)
  expect(barMedia()).toContainEqual(['dark', 'all'])
})

test('a theme is set for this visit when the browser keeps nothing', () => {
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('The quota has been exceeded.', 'QuotaExceededError')
  })

  setTheme('dark')

  expect(getTheme()).toBe('dark')
  expect(root.classList.contains('dark')).toBe(true)
  expect(localStorage.getItem('tdeemon.theme')).toBeNull()
})
