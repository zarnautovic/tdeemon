import { expect, test } from 'vitest'

import type { Item } from '@/api/types'
import { mealName, slotAt } from '@/components/day/meal-slot'
import { en } from '@/i18n/en'
import { hr } from '@/i18n/hr'

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

// itemsOf are items of these names, the first a food's of a brand.
function itemsOf(...names: string[]): Item[] {
  return names.map((name, i) => {
    const item: Item = {
      id: `0193c3a5-0000-7000-8000-0000000000b${String(i)}`,
      name,
      quantity: { amount: 150, unit: 'g' },
      amount: 150,
      unit: 'g',
      nutrients: { kcal: 100, protein: 10, carbs: 10, fat: 2 },
      source: 'label',
    }
    if (i === 0) {
      item.food_id = '0193c3a5-0000-7000-8000-0000000000f1'
      item.brand = 'Dukat'
    }
    return item
  })
}

test("a meal's name is its short description, or else what was in it, never the hour's meal", () => {
  const skyr = itemsOf('Skyr vanilla', 'banana', 'honey')
  expect(mealName({ description: 'Pasta', items: skyr }, en)).toBe('Pasta')
  expect(mealName({ description: 'x'.repeat(40), items: skyr }, en)).toBe('x'.repeat(40))
  // Its items by their names, without the brand.
  expect(mealName({ description: '', items: skyr.slice(0, 1) }, en)).toBe('Skyr vanilla')
  expect(mealName({ description: '', items: skyr.slice(0, 2) }, en)).toBe('Skyr vanilla and banana')
  expect(mealName({ description: '', items: skyr }, en)).toBe('Skyr vanilla + 2')
  // A model's long list goes under the name, which is what was in it.
  expect(mealName({ description: 'x'.repeat(41), items: skyr }, en)).toBe('Skyr vanilla + 2')
  // In the language, the import's rest of a meal too.
  expect(mealName({ description: '', items: itemsOf('Skyr vanilija', 'banana') }, hr)).toBe(
    'Skyr vanilija i banana',
  )
  expect(
    mealName(
      { description: '', items: itemsOf('Rest of the meal (history split)', 'Juha', 'Kruh') },
      hr,
    ),
  ).toBe('Ostatak obroka + 2')
})
