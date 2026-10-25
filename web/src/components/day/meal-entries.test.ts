import { expect, test } from 'vitest'

import type { Food, Item, Meal } from '@/api/types'
import {
  amountOf,
  converted,
  foodEntry,
  inputOf,
  inUnit,
  itemOf,
  repeatOf,
  tooMuch,
  unitGrams,
  unitIn,
  unitsOf,
  type Entry,
} from '@/components/day/meal-entries'

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
  const at = {
    at: '2026-10-21T07:00:00Z',
    day: '2026-10-21',
    meal_id: bread.id,
    amount: 80,
    unit: 'g' as const,
  }
  const slices = foodEntry(
    { ...bread, last_use: { ...at, quantity: { serving: 'slice', servings: 2 } } },
    field,
  )
  expect(slices).toMatchObject({ amount: '2', unit: 'serving:slice' })
  const grams = foodEntry({ ...bread, last_use: { ...at, quantity: { amount: 80 } } }, field)
  expect(grams).toMatchObject({ amount: '80', unit: 'g' })
  // Servings without their name, of the food's one: its amount.
  const unnamed = foodEntry({ ...bread, last_use: { ...at, quantity: { servings: 2 } } }, field)
  expect(unnamed).toMatchObject({ amount: '80', unit: 'g' })
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
  // Nothing of a food is no item: the server refuses it.
  expect(itemOf(entry('g', '0'), parse)).toBeUndefined()
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

// item is an item of a food of the user's, given as quantity.
function item(quantity: Item['quantity'], over: Partial<Item> = {}): Item {
  return {
    id: '0193c3a5-0000-7000-8000-0000000000b1',
    food_id: bread.id,
    name: 'Rye bread',
    quantity,
    amount: 80,
    unit: 'g',
    nutrients: { kcal: 207, protein: 7, carbs: 38, fat: 3 },
    source: 'label',
    ...over,
  }
}

test('an item logged again is given as it was, a food by its id', () => {
  expect(inputOf(item({ amount: 80, unit: 'g' }))).toEqual({
    food_id: bread.id,
    amount: 80,
    unit: 'g',
  })
  // Without a unit, the food's, which stays the food's.
  expect(inputOf(item({ amount: 80 }))).toEqual({ food_id: bread.id, amount: 80 })
  expect(inputOf(item({ serving: 'slice', servings: 2 }, { note: 'toasted' }))).toEqual({
    food_id: bread.id,
    note: 'toasted',
    serving: 'slice',
    servings: 2,
  })
  expect(inputOf(item({ portions: 1.5 }, { food_id: stew.id, unit: 'portion' }))).toEqual({
    food_id: stew.id,
    portions: 1.5,
  })
})

test("servings without their name, of the food's one, are its amount in its unit", () => {
  const amount = amountOf({ ...item({ servings: 2 }, { note: 'toasted' }), food_id: bread.id })
  expect(amount?.n).toBe(80)
  expect(amount?.unit).toBe('g')
  expect(amount?.with({ ...item({ servings: 2 }), food_id: bread.id }, 120)).toEqual({
    food_id: bread.id,
    amount: 120,
    unit: 'g',
  })
})

test('an item given without a quantity is logged again in its amount and unit', () => {
  expect(inputOf(item({}))).toEqual({ food_id: bread.id, amount: 80, unit: 'g' })
  expect(inputOf(item({}, { amount: 2, unit: 'portion', note: 'big' }))).toEqual({
    food_id: bread.id,
    portions: 2,
    note: 'big',
  })
})

test('an estimate is logged again with all its values and its note', () => {
  const nutrients = { kcal: 250, protein: 8, carbs: 12, fat: 18, fiber: 4, salt: 1.2 }
  const { food_id: _food, ...ofFood } = item({ portions: 1 })
  const salad: Item = { ...ofFood, name: 'Salad', nutrients, source: 'estimate', note: 'canteen' }
  expect(inputOf(salad)).toEqual({ name: 'Salad', nutrients, note: 'canteen' })
})

test('a meal repeated is a new meal of its items, with its words', () => {
  const meal: Meal = {
    id: '0193c3a5-0000-7000-8000-0000000000a1',
    eaten_at: '2026-10-22T07:00:00Z',
    description: 'Breakfast',
    note: 'before the run',
    client: 'mcp',
    total: { kcal: 207, protein: 7, carbs: 38, fat: 3 },
    items: [item({ amount: 80, unit: 'g' })],
  }
  const first = repeatOf(meal)
  // Under an id made now (UUIDv7), and eaten now: the server takes the
  // time of the id.
  expect(first).toEqual({
    id: expect.stringMatching(/^[0-9a-f]{8}-[0-9a-f]{4}-7/) as unknown,
    description: 'Breakfast',
    note: 'before the run',
    items: [{ food_id: bread.id, amount: 80, unit: 'g' }],
  })
  expect(repeatOf(meal).id).not.toBe(first.id)
  // Words it does not have are left out, not sent empty.
  const { note: _note, ...noNote } = meal
  const bare = repeatOf({ ...noNote, description: '' })
  expect('description' in bare || 'note' in bare).toBe(false)
})

test("a unit's grams are a serving's, a portion's when the food says them, or one", () => {
  expect(unitGrams(bread, 'serving:slice')).toBe(40)
  expect(unitGrams(bread, 'serving:loaf')).toBeUndefined()
  expect(unitGrams(bread, 'g')).toBe(1)
  // Grams of a food in millilitres are not millilitres: no density.
  expect(unitGrams(bread, 'ml')).toBeUndefined()
  expect(unitGrams({ ...bread, basis: '100ml' }, 'g')).toBeUndefined()
  expect(unitGrams({ ...bread, basis: '100ml' }, 'ml')).toBe(1)
  expect(unitGrams(stew, 'g')).toBe(1)
  expect(unitGrams(stew, 'portion')).toBe(450)
  const { portion_grams: _grams, ...noGrams } = stew
  expect(unitGrams(noGrams, 'portion')).toBeUndefined()
})

test('a serving is found as the server finds it, whatever its case and spaces', () => {
  const loaf = { ...bread, servings: [{ name: 'Big  slice', amount: 60 }] }
  expect(unitIn(loaf, 'serving:big slice')).toBe('serving:Big  slice')
  expect(unitGrams(loaf, 'serving:BIG SLICE')).toBe(60)
  expect(unitIn(loaf, 'serving:crust')).toBe('serving:crust')
  expect(unitIn(loaf, 'g')).toBe('g')
  // Folded as the server folds case: ẞ and ß are ss.
  const piece = { ...bread, servings: [{ name: 'Großes Stück', amount: 120 }] }
  expect(unitGrams(piece, 'serving:GROẞES STÜCK')).toBe(120)
  expect(unitGrams(piece, 'serving:grosses stück')).toBe(120)
  // A ligature folds to its letters, a final sigma to a sigma.
  const cup = {
    ...bread,
    servings: [
      { name: 'ﬂ oz', amount: 29.5735 },
      { name: 'κουπας', amount: 250 },
    ],
  }
  expect(unitGrams(cup, 'serving:FL OZ')).toBe(29.5735)
  expect(unitGrams(cup, 'serving:ΚΟΥΠΑΣ')).toBe(250)
  // I and ı are two to the server, and so here.
  const dots = {
    ...bread,
    servings: [
      { name: 'I', amount: 10 },
      { name: 'ı', amount: 90 },
    ],
  }
  expect(unitGrams(dots, 'serving:ı')).toBe(90)
  expect(unitGrams(dots, 'serving:i')).toBe(10)
  // A micro sign is a mu to the server.
  const micro = { ...bread, servings: [{ name: 'µg', amount: 0.000001 }] }
  expect(unitIn(micro, 'serving:μg')).toBe('serving:µg')
})

test("a food's entry in another unit weighs what it did, both ways", () => {
  const nuts: Food = { ...bread, name: 'Nuts', servings: [{ name: 'handful', amount: 30 }] }
  const grams = { key: 'k', kind: 'food' as const, food: nuts, amount: '100', unit: 'g' }
  const handfuls = inUnit(grams, 'serving:handful', parse, field)
  expect(handfuls).toMatchObject({ unit: 'serving:handful', amount: '3.33', exact: 100 / 30 })
  // Sent as worked out, not as shown.
  expect(itemOf(handfuls, parse)).toMatchObject({ serving: 'handful', servings: 100 / 30 })
  const back = inUnit(handfuls, 'g', parse, field)
  expect(back).toMatchObject({ unit: 'g', amount: '100', exact: 100 })
  expect(itemOf(back, parse)).toMatchObject({ amount: 100, unit: 'g' })
  // Typed, the amount is the one typed.
  const typed = inUnit({ ...grams, unit: 'serving:handful', amount: '2' }, 'g', parse, field)
  expect(typed).toMatchObject({ amount: '60', exact: 60 })
})

test("a food's entry in a unit of no grams asks for its amount anew", () => {
  const stewGrams = { key: 'k', kind: 'food' as const, food: stew, amount: '1', unit: 'portion' }
  const { portion_grams: _grams, ...ungrammed } = stew
  const noGrams = { ...stewGrams, food: ungrammed }
  expect(inUnit(stewGrams, 'g', parse, field)).toMatchObject({ amount: '450', exact: 450 })
  const asked = inUnit(noGrams, 'g', parse, field)
  expect(asked).toEqual({ key: 'k', kind: 'food', food: noGrams.food, amount: '', unit: 'g' })
  expect(inUnit({ ...stewGrams, amount: 'a lot' }, 'g', parse, field)).toMatchObject({
    amount: '',
  })
  expect(converted(bread, 'g', 'ml', 100)).toBeUndefined()
})

test('an item holds no more than the server takes', () => {
  // Grams, servings and their grams, portions, and a portion's grams.
  expect(tooMuch(bread, 'g', 10_000)).toBe(false)
  expect(tooMuch(bread, 'g', 10_001)).toBe(true)
  expect(tooMuch(bread, 'serving:slice', 100)).toBe(false)
  expect(tooMuch(bread, 'serving:slice', 101)).toBe(true)
  const sack = { ...bread, servings: [{ name: 'sack', amount: 5000 }] }
  expect(tooMuch(sack, 'serving:sack', 2)).toBe(false)
  expect(tooMuch(sack, 'serving:sack', 2.5)).toBe(true)
  expect(tooMuch(stew, 'portion', 50)).toBe(false)
  expect(tooMuch(stew, 'portion', 51)).toBe(true)
  // Grams of a food by the portion: no more than the most grams either.
  expect(tooMuch(stew, 'g', 10_000)).toBe(false)
  expect(tooMuch(stew, 'g', 10_001)).toBe(true)
  const bite = { ...stew, portion_grams: 100 }
  expect(tooMuch(bite, 'g', 5000)).toBe(false)
  expect(tooMuch(bite, 'g', 5100)).toBe(true)
  // A food not known yet: the count of servings alone.
  expect(tooMuch(undefined, 'serving:slice', 101)).toBe(true)

  const entry = foodEntry(bread, field)
  if (entry.kind !== 'food') {
    throw new Error('a food gives no food entry')
  }
  expect(itemOf({ ...entry, unit: 'serving:slice', amount: '99999' }, parse)).toBeUndefined()
  const plate = {
    key: '0193c3a5-0000-7000-8000-0000000000e1',
    kind: 'estimate' as const,
    name: 'Feast',
    kcal: '10001',
    protein: '1',
    carbs: '1',
    fat: '1',
  }
  expect(itemOf(plate, parse)).toBeUndefined()
  expect(itemOf({ ...plate, kcal: '10000' }, parse)).toBeDefined()
})
