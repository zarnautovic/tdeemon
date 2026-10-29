import { expect, test } from 'vitest'

import { clockAt, instantAt, mealInstant, nightAfter, parseClock, toCome } from '@/lib/zoned'

test.each([
  ['2026-10-23', '08:45', 'America/Los_Angeles', '2026-10-23T15:45:00.000Z'],
  ['2026-01-15', '08:45', 'America/Los_Angeles', '2026-01-15T16:45:00.000Z'],
  ['2026-07-01', '12:00', 'Europe/Zagreb', '2026-07-01T10:00:00.000Z'],
  ['2026-12-31', '23:30', 'Europe/Zagreb', '2026-12-31T22:30:00.000Z'],
  ['2026-10-23', '00:00', 'UTC', '2026-10-23T00:00:00.000Z'],
  // Summer time begins in Zagreb at 02:00 on March 29: 02:30 is skipped.
  ['2026-03-29', '02:30', 'Europe/Zagreb', '2026-03-29T01:30:00.000Z'],
  // It ends at 03:00 on October 25: 02:30 comes twice, the second is taken.
  ['2026-10-25', '02:30', 'Europe/Zagreb', '2026-10-25T01:30:00.000Z'],
  // West of UTC the other way: in Los Angeles 02:30 on March 8 is skipped
  // and comes an hour before, 01:30 on November 1 twice and is the first.
  ['2026-03-08', '02:30', 'America/Los_Angeles', '2026-03-08T09:30:00.000Z'],
  ['2026-11-01', '01:30', 'America/Los_Angeles', '2026-11-01T08:30:00.000Z'],
])('%s %s in %s is %s', (date, time, zone, want) => {
  expect(instantAt(date, time, zone)).toBe(want)
})

test('a time before the day start hour is of the night after the day', () => {
  expect(mealInstant('2026-10-22', '01:30', 'Europe/Zagreb', 4)).toBe('2026-10-22T23:30:00.000Z')
  expect(mealInstant('2026-10-22', '04:00', 'Europe/Zagreb', 4)).toBe('2026-10-22T02:00:00.000Z')
})

test('a time before the day start hour is of the night after, past the summer time it ends', () => {
  expect(nightAfter('03:59', 4)).toBe(true)
  expect(nightAfter('04:00', 4)).toBe(false)
  expect(nightAfter('00:00', 0)).toBe(false)
  // Summer time ends in Zagreb at 03:00 on October 25: 02:00 of the day of
  // the 24th comes twice, and the second, in winter time, is taken.
  expect(mealInstant('2026-10-24', '02:00', 'Europe/Zagreb', 4)).toBe('2026-10-25T01:00:00.000Z')
})

test('of a time the clock shows twice, the one gone by now, while the other is to come', () => {
  // At 02:30 in summer time, 02:10 in summer time is gone, in winter
  // time to come.
  const firstHalfHour = Date.parse('2026-10-25T00:30:00Z')
  expect(mealInstant('2026-10-24', '02:10', 'Europe/Zagreb', 4, firstHalfHour)).toBe(
    '2026-10-25T00:10:00.000Z',
  )
  // Both gone, or both to come: the second, as instantAt takes it.
  expect(
    mealInstant('2026-10-24', '02:10', 'Europe/Zagreb', 4, Date.parse('2026-10-25T05:00:00Z')),
  ).toBe('2026-10-25T01:10:00.000Z')
  expect(mealInstant('2026-10-24', '02:40', 'Europe/Zagreb', 4, firstHalfHour)).toBe(
    '2026-10-25T01:40:00.000Z',
  )
  // Far east of UTC too: summer time ends in Auckland at 03:00 on April 5.
  expect(
    mealInstant('2026-04-04', '02:30', 'Pacific/Auckland', 4, Date.parse('2026-04-04T13:40:00Z')),
  ).toBe('2026-04-04T13:30:00.000Z')
  // West of UTC the first is taken, and gone before the second.
  expect(
    mealInstant(
      '2026-10-31',
      '01:30',
      'America/Los_Angeles',
      4,
      Date.parse('2026-11-01T08:45:00Z'),
    ),
  ).toBe('2026-11-01T08:30:00.000Z')
  // A time the clock skips has no other.
  expect(
    mealInstant('2026-03-28', '02:30', 'Europe/Zagreb', 4, Date.parse('2026-03-29T00:00:00Z')),
  ).toBe('2026-03-29T01:30:00.000Z')
})

test("a time still to come, past a clock's few minutes ahead", () => {
  const now = Date.parse('2026-10-24T14:00:00Z')
  expect(toCome('2026-10-24T14:05:00.000Z', now)).toBe(false)
  expect(toCome('2026-10-24T14:05:01.000Z', now)).toBe(true)
  expect(toCome('2026-10-25T01:00:00.000Z', now)).toBe(true)
  expect(toCome('2026-10-23T23:00:00.000Z', now)).toBe(false)
})

test("a clock's time at an instant", () => {
  expect(clockAt(Date.parse('2026-10-23T15:45:30Z'), 'America/Los_Angeles')).toBe('08:45')
})

test('what is no date or time is refused', () => {
  expect(() => instantAt('2026-10-23', '8:45', 'UTC')).toThrow(RangeError)
})

test.each([
  ['19:30', '19:30'],
  ['19.30', '19:30'],
  ['19,30', '19:30'],
  ['1930', '19:30'],
  ['7:05', '07:05'],
  ['7', '07:00'],
  [' 08:15 ', '08:15'],
  ['0', '00:00'],
  ['123', '01:23'],
  // On a 12-hour clock, as English writes it, and as it is typed.
  ['7:30 PM', '19:30'],
  ['7:30 PM', '19:30'],
  ['7.30pm', '19:30'],
  ['7 p.m.', '19:00'],
  ['11:59 PM', '23:59'],
  ['12 PM', '12:00'],
  ['12 AM', '00:00'],
  ['12:15 am', '00:15'],
  ['9:05 a.m.', '09:05'],
])('%j is the time %s', (typed, time) => {
  expect(parseClock(typed)).toBe(time)
})

test.each([
  '',
  '24:00',
  '19:60',
  '7:5',
  'noon',
  '19:30:00',
  '12345',
  '13 PM',
  '19:30 PM',
  '0 AM',
  '7:30 XM',
  'PM',
])('%j is no time', (typed) => {
  expect(parseClock(typed)).toBeUndefined()
})
