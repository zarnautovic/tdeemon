import { expect, test } from 'vitest'

import type { Item, Meal } from '@/api/types'
import { pastMeals } from '@/components/day/past-meals'

const oats = '0193c3a5-0000-7000-8000-0000000000f1'
const yoghurt = '0193c3a5-0000-7000-8000-0000000000f2'
const banana = '0193c3a5-0000-7000-8000-0000000000f3'

// item is an item of food (an estimate without one), of grams.
function item(food: string | undefined, grams: number, name = 'Food'): Item {
  const nutrients = { kcal: grams, protein: 0, carbs: 0, fat: 0 }
  return {
    id: `${food ?? name}-${String(grams)}`,
    ...(food === undefined ? {} : { food_id: food }),
    name,
    quantity: { amount: grams, unit: 'g' },
    amount: grams,
    unit: 'g',
    nutrients,
    source: food === undefined ? 'estimate' : 'label',
  }
}

function meal(id: string, description: string, items: Item[]): Meal {
  return {
    id,
    eaten_at: '2026-10-22T07:00:00Z',
    description,
    client: 'web',
    total: { kcal: 0, protein: 0, carbs: 0, fat: 0 },
    items,
  }
}

const ids = (meals: Meal[]) => meals.map((m) => m.id)

test('the meals of the same foods show once, the latest, whatever the amounts and names', () => {
  const meals = [
    meal('latest', 'Oats with yoghurt', [item(oats, 60), item(yoghurt, 150), item(banana, 120)]),
    meal('more', 'Oats with yoghurt', [item(oats, 80), item(yoghurt, 200), item(banana, 100)]),
    // In another order, and named otherwise.
    meal('other order', 'Breakfast', [item(banana, 120), item(oats, 60), item(yoghurt, 150)]),
    // Without the banana: another meal.
    meal('no banana', 'Oats with yoghurt', [item(oats, 60), item(yoghurt, 150)]),
    // The yoghurt twice is not once.
    meal('twice', '', [item(oats, 60), item(yoghurt, 150), item(yoghurt, 50)]),
  ]

  expect(ids(pastMeals(meals))).toEqual(['latest', 'no banana', 'twice'])
})

test('an estimate is told by its name, whatever its values', () => {
  const meals = [
    meal('pizza', '', [item(undefined, 800, 'Pizza')]),
    meal('bigger pizza', '', [item(undefined, 1000, 'Pizza')]),
    meal('stew', '', [item(undefined, 800, 'Stew')]),
    // A food of the same name is not the estimate.
    meal('food', '', [item(oats, 800, 'Pizza')]),
  ]

  expect(ids(pastMeals(meals))).toEqual(['pizza', 'stew', 'food'])
})
