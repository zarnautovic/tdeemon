import { expect, test } from 'vitest'

import type { Food } from '@/api/types'
import { foodEntry, itemOf, unitsOf, type Entry } from '@/components/day/meal-entries'

const field = (n: number) => String(n)
const parse = (s: string) => (/^\d+(\.\d+)?$/.test(s) ? Number(s) : undefined)

const bread: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000f1',
  name: 'Rye bread',
  basis: '100g',
  source: 'label',
  archived: false,
  nutrients: { kcal: 259, protein: 8.5, carbs: 48, fat: 3.3 },
  servings: [{ name: 'slice', amount: 40 }],
}
const stew: Food = {
  ...bread,
  id: '0193c3a5-0000-7000-8000-0000000000f2',
  name: 'Stew',
  basis: 'portion',
  portion_grams: 450,
  servings: [],
}

test("a food's units are its basis's, a portion's grams and its servings", () => {
  expect(unitsOf(bread)).toEqual(['g', 'serving:slice'])
  expect(unitsOf(stew)).toEqual(['portion', 'g'])
  expect(unitsOf({ ...bread, basis: '100ml', servings: [] })).toEqual(['ml'])
})

test('a food comes in the amount it was eaten in last, or 100 of its unit, or a portion', () => {
  const at = { at: '2026-10-21T07:00:00Z', meal_id: bread.id, amount: 80, unit: 'g' as const }
  const slices = foodEntry(
    { ...bread, last_use: { ...at, quantity: { serving: 'slice', servings: 2 } } },
    field,
  )
  expect(slices).toMatchObject({ amount: '2', unit: 'serving:slice' })
  const grams = foodEntry({ ...bread, last_use: { ...at, quantity: { amount: 80 } } }, field)
  expect(grams).toMatchObject({ amount: '80', unit: 'g' })
  // A serving the food no longer has is not offered.
  const gone = foodEntry(
    { ...bread, last_use: { ...at, quantity: { serving: 'loaf', servings: 1 } } },
    field,
  )
  expect(gone).toMatchObject({ amount: '100', unit: 'g' })
  expect(foodEntry(stew, field)).toMatchObject({ amount: '1', unit: 'portion' })
})

test('an entry sends its item, or nothing while a number is not one', () => {
  const entry = (unit: string, amount: string): Entry => ({
    key: 'k',
    kind: 'food',
    food: bread,
    amount,
    unit,
  })
  expect(itemOf(entry('g', '120'), parse)).toEqual({
    id: 'k',
    food_id: bread.id,
    amount: 120,
    unit: 'g',
  })
  expect(itemOf(entry('serving:slice', '2'), parse)).toEqual({
    id: 'k',
    food_id: bread.id,
    serving: 'slice',
    servings: 2,
  })
  expect(itemOf(entry('portion', '1.5'), parse)).toEqual({
    id: 'k',
    food_id: bread.id,
    portions: 1.5,
  })
  expect(itemOf(entry('g', 'lots'), parse)).toBeUndefined()
  const plate: Entry = {
    key: 'p',
    kind: 'estimate',
    name: ' Pizza ',
    kcal: '850',
    protein: '32',
    carbs: '105',
    fat: '30',
  }
  expect(itemOf(plate, parse)).toEqual({
    id: 'p',
    name: 'Pizza',
    nutrients: { kcal: 850, protein: 32, carbs: 105, fat: 30 },
  })
  expect(itemOf({ ...plate, name: ' ' }, parse)).toBeUndefined()
  expect(itemOf({ ...plate, fat: '' }, parse)).toBeUndefined()
})
