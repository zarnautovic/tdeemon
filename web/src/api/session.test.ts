import { expect, test } from 'vitest'

import { isStatus, safeTarget } from '@/api/session'

test('a path on the app is a target after signing in', () => {
  expect(safeTarget('/foods')).toBe('/foods')
  expect(safeTarget('/foods?query=egg#top')).toBe('/foods?query=egg#top')
  expect(safeTarget('/logins')).toBe('/logins')
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
    '/login',
    '/login?redirect=%2F',
    '/login/',
  ]) {
    expect(safeTarget(target)).toBe('/')
  }
})

test("a problem's status tells what failed", () => {
  expect(isStatus({ status: 401, title: 'Unauthorized' }, 401)).toBe(true)
  expect(isStatus({ status: 500 }, 401)).toBe(false)
  expect(isStatus(new TypeError('Failed to fetch'), 401)).toBe(false)
  expect(isStatus(null, 401)).toBe(false)
})
