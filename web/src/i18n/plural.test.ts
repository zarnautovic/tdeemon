import { expect, test } from 'vitest'

import { plural } from '@/i18n/plural'

test("a count takes its language's form", () => {
  const minutes = { one: 'minutu', few: 'minute', other: 'minuta' }
  expect([1, 2, 5, 11, 21, 22, 25].map((n) => plural('hr', n, minutes))).toEqual([
    'minutu',
    'minute',
    'minuta',
    'minuta',
    'minutu',
    'minute',
    'minuta',
  ])
  expect([1, 2].map((n) => plural('en', n, { one: 'minute', other: 'minutes' }))).toEqual([
    'minute',
    'minutes',
  ])
})

test('a form the word lacks is its other', () => {
  expect(plural('hr', 2, { other: 'puta' })).toBe('puta')
})
