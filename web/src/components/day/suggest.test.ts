import { expect, test } from 'vitest'

import type { Food, RecentMeal } from '@/api/types'
import { suggestFoods, suggestMeals, type Moment } from '@/components/day/suggest'

// meal is a meal of before of these foods, eaten at these times (the
// latest first), each of its day on the user's clock.
function meal(id: string, foods: string[], ...times: [at: string, day: string][]): RecentMeal {
  return {
    id,
    eaten_at: times[0]?.[0] ?? '',
    description: id,
    client: 'web',
    total: { kcal: 0, protein: 0, carbs: 0, fat: 0 },
    items: foods.map((of) => ({
      id: `${id} ${of}`,
      food_id: of,
      name: of,
      quantity: { amount: 100, unit: 'g' },
      amount: 100,
      unit: 'g',
      nutrients: { kcal: 0, protein: 0, carbs: 0, fat: 0 },
      source: 'label',
    })),
    eaten: times.map(([at, day]) => ({ at, day })),
  }
}

// food is a food of the user's, last used on day.
function food(id: string, day: string): Food {
  return {
    id,
    name: id,
    basis: '100g',
    source: 'label',
    archived: false,
    nutrients: { kcal: 0, protein: 0, carbs: 0, fat: 0 },
    last_use: {
      at: `${day}T12:00:00Z`,
      day,
      meal_id: 'm',
      quantity: { amount: 100, unit: 'g' },
      amount: 100,
      unit: 'g',
    },
  }
}

// 20:00 on the 23rd in Los Angeles: dinner, on the 23rd.
const dinner: Moment = {
  slot: 'dinner',
  now: Date.parse('2026-10-24T03:00:00Z'),
  timeZone: 'America/Los_Angeles',
  date: '2026-10-23',
}

// The meals of before, the latest first.
const meals = [
  // Just logged, at lunch today, and at dinner each day before.
  meal(
    'today',
    ['tuna'],
    ['2026-10-23T20:32:00Z', '2026-10-23'],
    ['2026-10-23T02:00:00Z', '2026-10-22'],
    ['2026-10-22T02:00:00Z', '2026-10-21'],
  ),
  meal('lunch', ['bread'], ['2026-10-22T19:00:00Z', '2026-10-22']),
  meal('pasta', ['pasta', 'oil'], ['2026-10-22T03:00:00Z', '2026-10-21']),
  meal(
    'stew',
    ['beef', 'oil'],
    ['2026-10-20T03:00:00Z', '2026-10-19'],
    ['2026-10-19T03:00:00Z', '2026-10-18'],
  ),
  // Dinners 13 and 15 days before: one counts.
  meal(
    'goulash',
    ['beef'],
    ['2026-10-11T04:00:00Z', '2026-10-10'],
    ['2026-10-09T03:00:00Z', '2026-10-08'],
  ),
  meal('old', ['bread'], ['2026-10-08T03:00:00Z', '2026-10-07']),
]

test("the meals of the meal of the day come first, the most eaten first, and the day's last", () => {
  const ranked = suggestMeals(meals, dinner)
  expect(ranked.meals.map((m) => m.id)).toEqual([
    'stew',
    'pasta',
    'goulash',
    'lunch',
    'old',
    'today',
  ])
  expect(ranked.ofSlot).toBe(3)
  // At lunch, the lunch alone; the day's still last.
  const lunch = suggestMeals(meals, { ...dinner, slot: 'lunch' })
  expect(lunch.meals.map((m) => m.id)).toEqual([
    'lunch',
    'pasta',
    'stew',
    'goulash',
    'old',
    'today',
  ])
  expect(lunch.ofSlot).toBe(1)
  // A day before, the meals of the 22nd were had, not today's.
  const before = suggestMeals(meals, { ...dinner, date: '2026-10-22' })
  expect(before.meals.map((m) => m.id)).toEqual([
    'stew',
    'pasta',
    'goulash',
    'old',
    'today',
    'lunch',
  ])
})

test('the foods of the meal of the day come first, by the times the meals had them', () => {
  // The oil of the pasta and the stew, three dinners; the beef of the
  // stew and the goulash, three; the pasta's, one. The tuna was had today,
  // and the skyr, by its last use, with no meal of before.
  const foods = [
    ...['bread', 'tuna', 'oil', 'pasta', 'beef'].map((id) => food(id, '2026-10-22')),
    food('skyr', '2026-10-23'),
  ]
  expect(suggestFoods(foods, meals, dinner).map((f) => f.id)).toEqual([
    'oil',
    'beef',
    'pasta',
    'bread',
    'tuna',
    'skyr',
  ])
})
