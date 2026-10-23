import { expect, test } from 'vitest'

import { safeTarget } from '@/api/session'

test('a path on the app is a target after signing in', () => {
  expect(safeTarget('/foods')).toBe('/foods')
  expect(safeTarget('/foods?query=egg#top')).toBe('/foods?query=egg#top')
  expect(safeTarget('/logins')).toBe('/logins')
  expect(safeTarget(`/${'a'.repeat(1023)}`)).toHaveLength(1024)
})

test('anything else is the start page', () => {
  for (const target of [
    undefined,
    42,
    '',
    'foods',
    'https://example.com/',
    '//example.com/',
    '/\\example.com',
    '/foods\\x',
    '/\t/example.com',
    '/foods\nx',
    '/foods\u007f',
    `/${'a'.repeat(1024)}`,
    '/login',
    '/LOGIN',
    '/login?redirect=%2F',
    '/login/',
  ]) {
    expect(safeTarget(target)).toBe('/')
  }
})
