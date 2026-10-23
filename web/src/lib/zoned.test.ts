import { expect, test } from 'vitest'

import { clockAt, instantAt, mealInstant } from '@/lib/zoned'

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

test("a clock's time at an instant", () => {
  expect(clockAt(Date.parse('2026-10-23T15:45:30Z'), 'America/Los_Angeles')).toBe('08:45')
})

test('what is no date or time is refused', () => {
  expect(() => instantAt('2026-10-23', '8:45', 'UTC')).toThrow(RangeError)
})
