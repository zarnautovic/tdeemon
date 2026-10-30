import { renderHook } from '@testing-library/react'
import { expect, test } from 'vitest'

import type { Ingredient } from '@/api/recipes'
import type { Food } from '@/api/types'
import { lastAmountOf, type FoodEntry } from '@/components/day/meal-entries'
import {
  dishOf,
  dishWrong,
  entryGrams,
  ingredientAmount,
  ingredientEntry,
  itemsOf,
  portionsOf,
  yieldOf,
} from '@/components/recipes/recipe-values'
import { potOf, weighingOf, weighingWrong, withPot } from '@/components/recipes/weighing'
import { en } from '@/i18n/en'
import { useFormat } from '@/i18n/format'

// The app's formats, in English.
const f = renderHook(() => useFormat()).result.current
const { parse } = f
const field = (n: number) => f.field(n)

const rice: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000f1',
  name: 'Basmati rice',
  basis: '100g',
  source: 'label',
  archived: false,
  nutrients: { kcal: 350, protein: 7, carbs: 78, fat: 1 },
  servings: [{ name: 'cup', amount: 180 }],
}

const egg: Food = {
  id: '0193c3a5-0000-7000-8000-0000000000f2',
  name: 'Egg',
  basis: 'portion',
  source: 'label',
  archived: false,
  nutrients: { kcal: 70, protein: 6, carbs: 0.5, fat: 5 },
  portion_grams: 55,
}

function entry(food: Food, amount: string, unit: string): FoodEntry {
  return { key: `${food.id}-${unit}`, kind: 'food', food, amount, unit }
}

test('an ingredient weighs as the server weighs it', () => {
  expect(entryGrams(entry(rice, '2', 'serving:cup'), parse)).toBe(360)
  expect(entryGrams(entry(rice, '', 'g'), parse)).toBe(0)
  // A food by the portion adds its grams only when given in grams.
  expect(entryGrams(entry(egg, '2', 'portion'), parse)).toBeUndefined()
  expect(entryGrams(entry(egg, '110', 'g'), parse)).toBe(110)
})

test('a dish comes to its ingredients, weighed or not, in portions or not', () => {
  const weighed = dishOf([entry(rice, '200', 'g')], parse, 500, 2)
  expect(weighed.total.kcal).toBe(700)
  expect(weighed.weight).toBe(500)
  expect(weighed.per100?.kcal).toBe(140)
  expect(weighed.portion?.kcal).toBe(350)
  expect(weighed.portionGrams).toBe(250)

  const raw = dishOf([entry(rice, '200', 'g')], parse, undefined, undefined)
  expect(raw.weighed).toBe(false)
  expect(raw.weight).toBe(200)
  expect(raw.per100?.kcal).toBe(350)
  expect(dishWrong(raw, 1)).toBeUndefined()

  // Without a weight, an egg in portions leaves the dish without grams.
  const eggs = dishOf([entry(egg, '2', 'portion')], parse, undefined, undefined)
  expect(eggs.weight).toBeUndefined()
  expect(dishWrong(eggs, 1)).toBe('noRaw')
  expect(dishWrong(dishOf([entry(rice, '10001', 'g')], parse, undefined, undefined), 1)).toBe(
    'heavy',
  )
})

test('a weight far off the raw ingredients is told, as the server tells it', () => {
  expect(yieldOf(250, 1000)).toEqual({ off: 'low', pct: 25 })
  expect(yieldOf(3100, 1000)?.off).toBe('high')
  expect(yieldOf(800, 1000)).toEqual({ off: undefined, pct: 80 })
  expect(yieldOf(undefined, 1000)).toBeUndefined()
})

test('portions are a whole number from 1 to 100', () => {
  expect(portionsOf('4', parse)).toBe(4)
  expect(portionsOf('0', parse)).toBeUndefined()
  expect(portionsOf('2.5', parse)).toBeUndefined()
  expect(portionsOf('101', parse)).toBeUndefined()
})

test('the pot gives the weight, and is remembered', () => {
  const w = weighingOf(field, 1200)
  expect(w).toEqual({ weight: '', pot: true, full: '', empty: '1200', cantWeigh: false })
  const full = withPot(w, { full: '2900' }, parse, field)
  expect(full.weight).toBe('1700')
  expect(weighingWrong(withPot(w, { full: '1000' }, parse, field), parse)).toEqual({ pot: true })
  expect(weighingWrong({ ...w, weight: '0' }, parse)).toEqual({ weight: true })
  expect(potOf({ ...w, empty: '' }, parse, 1200)).toBe(1200)
  expect(potOf({ ...w, empty: '900' }, parse, 1200)).toBe(900)
})

const portionOf: Ingredient = {
  food_id: '0193c3a5-0000-7000-8000-0000000000f3',
  name: 'Tomato sauce',
  quantity: { servings: 0.25, serving: 'recipe' },
  amount: 200,
  unit: 'g',
  nutrients: { kcal: 120, protein: 3, carbs: 18, fat: 4 },
  source: 'recipe',
}

test('an ingredient is said as it was given', () => {
  expect(ingredientAmount(portionOf, en, f)).toBe('¼ of the recipe (200 g)')
  expect(
    ingredientAmount({ ...portionOf, quantity: { servings: 2, serving: 'portion' } }, en, f),
  ).toBe('2 portions (200 g)')
  expect(
    ingredientAmount(
      { ...portionOf, source: 'label', quantity: { servings: 2, serving: 'cup' } },
      en,
      f,
    ),
  ).toBe('2 cup (200 g)')
  expect(
    ingredientAmount(
      { ...portionOf, quantity: { portions: 2 }, unit: 'portion', amount: 2 },
      en,
      f,
    ),
  ).toBe('2 portions')
  expect(ingredientAmount({ ...portionOf, quantity: { amount: 200, unit: 'g' } }, en, f)).toBe(
    '200 g',
  )
})

test('an ingredient starts its editor in the quantity it was given', () => {
  const cup = ingredientEntry(
    { ...portionOf, food_id: rice.id, source: 'label', quantity: { servings: 2, serving: 'Cup' } },
    rice,
    field,
  )
  expect([cup.amount, cup.unit]).toEqual(['2', 'serving:cup'])
  const eggs = ingredientEntry({ ...portionOf, quantity: { portions: 3 } }, egg, field)
  expect([eggs.amount, eggs.unit]).toEqual(['3', 'portion'])
  const grams = ingredientEntry({ ...portionOf, quantity: { amount: 150 } }, rice, field)
  expect([grams.amount, grams.unit]).toEqual(['150', 'g'])
})

test("a recipe's items keep their notes, and none while one is wrong", () => {
  const rows = [entry(rice, '200', 'g'), entry(egg, '2', 'portion')]
  const notes = new Map([[rows[1]?.key ?? '', 'free range']])
  expect(itemsOf(rows, notes, parse)).toEqual([
    { food_id: rice.id, amount: 200, unit: 'g' },
    { food_id: egg.id, portions: 2, note: 'free range' },
  ])
  expect(itemsOf([entry(rice, 'x', 'g')], notes, parse)).toBeUndefined()
})

// eatenAs is a recipe eaten last in a quantity.
function eatenAs(quantity: NonNullable<Food['last_use']>['quantity']): Food {
  return {
    ...rice,
    source: 'recipe',
    last_use: {
      at: '2026-10-21T12:00:00Z',
      day: '2026-10-21',
      meal_id: '0193c3a5-0000-7000-8000-0000000000a1',
      quantity,
      amount: 100,
      unit: 'g',
    },
  }
}

test('a recipe eaten last is said as a share of it, or portions', () => {
  expect(lastAmountOf(eatenAs({ servings: 0.1, serving: 'recipe' }), en, f)).toBe('⅒ of the recipe')
  expect(lastAmountOf(eatenAs({ servings: 1, serving: 'portion' }), en, f)).toBe('1 portion')
  expect(lastAmountOf(eatenAs({ amount: 100, unit: 'g' }), en, f)).toBe('100 g')
})
