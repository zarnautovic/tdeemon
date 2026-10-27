import { expect, test } from 'vitest'

import { mealName, slotAt } from '@/components/day/meal-slot'

// On the user's clock: 15:30 in Los Angeles is 22:30 in UTC.
test.each([
  ['2026-10-23T14:00:00Z', 'breakfast'], // 07:00
  ['2026-10-23T17:59:00Z', 'breakfast'], // 10:59
  ['2026-10-23T18:00:00Z', 'lunch'], // 11:00
  ['2026-10-23T21:59:00Z', 'lunch'], // 14:59
  ['2026-10-23T22:30:00Z', 'snack'], // 15:30
  ['2026-10-24T01:00:00Z', 'dinner'], // 18:00
  ['2026-10-24T08:30:00Z', 'dinner'], // 01:30, the night after
])('a meal at %s is %s', (at, slot) => {
  expect(slotAt(Date.parse(at), 'America/Los_Angeles')).toBe(slot)
})

test("a meal's name is its short description, or else the meal of the day it was eaten at", () => {
  const slots = { breakfast: 'Breakfast', lunch: 'Lunch', snack: 'Snack', dinner: 'Dinner' }
  const at = '2026-10-23T19:00:00Z' // 12:00 in Los Angeles
  const zone = 'America/Los_Angeles'
  expect(mealName({ description: 'Pasta', eaten_at: at }, zone, slots)).toBe('Pasta')
  expect(mealName({ description: '', eaten_at: at }, zone, slots)).toBe('Lunch')
  expect(mealName({ description: 'x'.repeat(41), eaten_at: at }, zone, slots)).toBe('Lunch')
  expect(mealName({ description: 'x'.repeat(40), eaten_at: at }, zone, slots)).toBe('x'.repeat(40))
})
