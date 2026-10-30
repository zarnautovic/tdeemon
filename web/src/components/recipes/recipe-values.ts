import type { Ingredient, RecipeWarning } from '@/api/recipes'
import type { components } from '@/api/schema'
import type { Food } from '@/api/types'
import {
  entryItem,
  entryValues,
  lastAmountOf,
  servingOf,
  servingsOf,
  unitGrams,
  unitIn,
  type FoodEntry,
  type Values,
} from '@/components/day/meal-entries'
import type { Messages } from '@/i18n/en'
import type { useFormat } from '@/i18n/format'
import limits from '@/lib/limits.json'
import { uuidv7 } from '@/lib/uuid'

// The servings of a recipe's food, by their names (package recipe): the
// whole dish, and one of its portions when it is divided.
export const wholeServing = 'recipe'
export const portionServing = 'portion'

// isRecipe tells a recipe's food, which comes with its recipe.
export function isRecipe(food: Pick<Food, 'source'>): boolean {
  return food.source === 'recipe'
}

const none: Values = { kcal: 0, protein: 0, carbs: 0, fat: 0 }

function plus(a: Values, b: Values): Values {
  return {
    kcal: a.kcal + b.kcal,
    protein: a.protein + b.protein,
    carbs: a.carbs + b.carbs,
    fat: a.fat + b.fat,
  }
}

function times(v: Values, k: number): Values {
  return { kcal: v.kcal * k, protein: v.protein * k, carbs: v.carbs * k, fat: v.fat * k }
}

// entryGrams is how many grams an ingredient adds to the raw weight, as
// the server weighs it (recipe.RawWeight): its grams or millilitres
// (taken as grams), or a serving's; of a food by the portion, only when
// given in grams. Undefined when it adds none it knows; 0 while its
// amount is not a number yet.
export function entryGrams(
  entry: FoodEntry,
  parse: (s: string) => number | undefined,
): number | undefined {
  const n = entry.exact ?? parse(entry.amount)
  if (n === undefined) {
    return 0
  }
  if (entry.food.basis === 'portion') {
    return entry.unit === 'g' ? n : undefined
  }
  const grams = entry.unit === 'ml' || entry.unit === 'g' ? 1 : unitGrams(entry.food, entry.unit)
  return grams === undefined ? undefined : n * grams
}

// Dish is a recipe's values as the server works them out of its
// ingredients and the dish's weight.
export interface Dish {
  // The whole dish's.
  total: Values
  // The raw ingredients' grams, when each one's is known.
  raw: number | undefined
  // The cooked dish's grams: its weight as weighed, else the raw
  // ingredients'; undefined when neither is known.
  weight: number | undefined
  weighed: boolean
  // Per 100 g of the cooked dish, when its weight is known.
  per100: Values | undefined
  portions: number | undefined
  // A portion's, and its grams, when divided.
  portion: Values | undefined
  portionGrams: number | undefined
}

// dishOf is what a recipe being written comes to: its ingredients'
// values, the dish's weight as weighed (yieldG, else the raw
// ingredients' stands for it) and its portions. An ingredient whose
// amount is not a number yet adds nothing.
export function dishOf(
  entries: readonly FoodEntry[],
  parse: (s: string) => number | undefined,
  yieldG: number | undefined,
  portions: number | undefined,
): Dish {
  let total = none
  let raw: number | undefined = 0
  for (const entry of entries) {
    total = plus(total, entryValues(entry, parse) ?? none)
    const grams = entryGrams(entry, parse)
    raw = raw === undefined || grams === undefined ? undefined : raw + grams
  }
  return dishFrom(total, raw, yieldG, portions)
}

// dishFrom is a dish of its whole values (total) and its raw
// ingredients' grams (raw, when known), weighed (yieldG) or not, in
// portions or not.
export function dishFrom(
  total: Values,
  raw: number | undefined,
  yieldG: number | undefined,
  portions: number | undefined,
): Dish {
  const weight = yieldG ?? (raw !== undefined && raw > 0 ? raw : undefined)
  return {
    total,
    raw,
    weight,
    weighed: yieldG !== undefined,
    per100: weight === undefined ? undefined : times(total, 100 / weight),
    portions,
    portion: portions === undefined ? undefined : times(total, 1 / portions),
    portionGrams: portions === undefined || weight === undefined ? undefined : weight / portions,
  }
}

// dishWrong is why a dish cannot be saved as it is: not weighed, and an
// ingredient adds no grams (noRaw) or the raw ingredients weigh more
// than a dish may (heavy, limits.json).
export function dishWrong(dish: Dish, entries: number): 'noRaw' | 'heavy' | undefined {
  if (dish.weighed || entries === 0) {
    return undefined
  }
  if (dish.raw === undefined) {
    return 'noRaw'
  }
  return dish.raw > limits.recipe.maxWeight ? 'heavy' : undefined
}

// YieldOff is how far the cooked dish's weight is off its raw
// ingredients', when far enough to warn of (limits.json, the server's
// recipe.LowYield and HighYield): low or high, and its share in percent.
export interface YieldOff {
  off: 'low' | 'high' | undefined
  pct: number
}

// yieldOf is the share of the raw ingredients' weight the cooked dish
// weighs; undefined when either is not known.
export function yieldOf(yieldG: number | undefined, raw: number | undefined): YieldOff | undefined {
  if (yieldG === undefined || raw === undefined || raw <= 0) {
    return undefined
  }
  const share = yieldG / raw
  return {
    off:
      share < limits.recipe.lowYield ? 'low' : share > limits.recipe.highYield ? 'high' : undefined,
    pct: share * 100,
  }
}

// The common fractions a share of a recipe is written as.
const fractions: readonly [number, string][] = [
  [1 / 2, '½'],
  [1 / 3, '⅓'],
  [2 / 3, '⅔'],
  [1 / 4, '¼'],
  [3 / 4, '¾'],
  [1 / 5, '⅕'],
  [1 / 6, '⅙'],
  [1 / 8, '⅛'],
  [1 / 10, '⅒'],
]

// shareOf is a share of a whole dish as a person writes it: a common
// fraction (¼), one of n (1/12), or else the number (number, 1.5).
export function shareOf(x: number, number: (n: number, digits?: number) => string): string {
  const common = fractions.find(([v]) => Math.abs(v - x) < 1e-6)
  if (common !== undefined) {
    return common[1]
  }
  const n = 1 / x
  if (x < 1 && Math.abs(n - Math.round(n)) < 1e-6 && Math.round(n) <= 100) {
    return `1/${String(Math.round(n))}`
  }
  return number(x, 2)
}

// servingName is a serving's name as the app says it: a recipe's food's
// own (the whole dish, a portion) in the language, in its count; any
// other as the food names it.
export function servingName(food: Pick<Food, 'source'>, name: string, n: number, t: Messages) {
  if (isRecipe(food) && (name === wholeServing || name === portionServing)) {
    return t.recipes.servings[name](n)
  }
  return name
}

type Format = ReturnType<typeof useFormat>

// ingredientAmount is how much of an ingredient a recipe has, as it was
// given: "200 g", "2 slice (60 g)", "1.5 portions", or of a recipe "¼ of
// the recipe (540 g)" and "2 portions (540 g)".
export function ingredientAmount(ingredient: Ingredient, t: Messages, f: Format): string {
  const q = ingredient.quantity
  const unit = ingredient.unit === 'portion' ? undefined : ingredient.unit
  const grams = unit === undefined ? '' : ` (${t.unit[unit](f.number(ingredient.amount))})`
  if (q.servings !== undefined && q.serving !== undefined) {
    if (ingredient.source === 'recipe' && q.serving === wholeServing) {
      return `${t.recipes.ofRecipe(shareOf(q.servings, f.number))}${grams}`
    }
    if (ingredient.source === 'recipe' && q.serving === portionServing) {
      return `${t.recipes.portions(f.number(q.servings, 2), q.servings)}${grams}`
    }
    return `${servingsOf(f.number(q.servings, 2), q.servings, q.serving)}${grams}`
  }
  if (q.portions !== undefined || unit === undefined) {
    const n = q.portions ?? ingredient.amount
    return t.day.portions(f.number(n, 2), n)
  }
  return t.unit[unit](f.number(ingredient.amount))
}

// entryUnitLabel is a unit of an ingredient's food in a menu: its grams
// or millilitres, a portion, or a serving by its name (servingName).
export function entryUnitLabel(food: Food, unit: string, n: number, t: Messages): string {
  if (unit === 'portion') {
    return t.log.portions(n)
  }
  const serving = servingOf(unit)
  return serving === undefined ? unit : servingName(food, serving, n, t)
}

// ingredientEntry is an ingredient of a recipe as its editor starts with
// it: its food as it is now (food), in the quantity it was given, its
// amount as field shows a number.
export function ingredientEntry(
  ingredient: Ingredient,
  food: Food,
  field: (n: number) => string,
): FoodEntry {
  const key = uuidv7()
  const q = ingredient.quantity
  if (q.servings !== undefined && q.serving !== undefined) {
    return {
      key,
      kind: 'food',
      food,
      amount: field(q.servings),
      unit: unitIn(food, `serving:${q.serving}`),
    }
  }
  if (q.portions !== undefined) {
    return { key, kind: 'food', food, amount: field(q.portions), unit: 'portion' }
  }
  if (q.amount !== undefined && q.servings === undefined) {
    return { key, kind: 'food', food, amount: field(q.amount), unit: q.unit ?? ingredient.unit }
  }
  return { key, kind: 'food', food, amount: field(ingredient.amount), unit: ingredient.unit }
}

// portionsOf is the portions typed, when a whole number of them a recipe
// may have; undefined for none, or for one it may not.
export function portionsOf(
  text: string,
  parse: (s: string) => number | undefined,
): number | undefined {
  const n = parse(text)
  return n !== undefined && Number.isInteger(n) && n >= 1 && n <= limits.recipe.maxPortions
    ? n
    : undefined
}

// itemsOf are a recipe's ingredients as a save sends them: each entry's
// item, without the id a meal's item has, with the note it had (notes, by
// the entry's key); undefined while one is wrong.
export function itemsOf(
  entries: readonly FoodEntry[],
  notes: ReadonlyMap<string, string>,
  parse: (s: string) => number | undefined,
): components['schemas']['ItemInput'][] | undefined {
  const items = []
  for (const entry of entries) {
    const checked = entryItem(entry, parse)
    if (!('item' in checked)) {
      return undefined
    }
    const { id: _id, ...item } = checked.item
    const note = notes.get(entry.key)
    items.push(note === undefined ? item : { ...item, note })
  }
  return items
}

// warningOf is what a warning of a recipe's save says, by its code.
export function warningOf(w: RecipeWarning, t: Messages): string {
  return w.code === 'yield_low' || w.code === 'yield_high'
    ? t.recipes.warnings[w.code]
    : t.meal.warnings[w.code]
}

// lastShareOf is how much of a recipe was eaten last, as it was given: a
// share of it, portions, or else as any food's (lastAmountOf).
export function lastShareOf(food: Food, t: Messages, f: Format): string | undefined {
  const q = food.last_use?.quantity
  if (q?.servings !== undefined && q.serving === wholeServing) {
    return t.recipes.ofRecipe(shareOf(q.servings, f.number))
  }
  if (q?.servings !== undefined && q.serving === portionServing) {
    return t.recipes.portions(f.number(q.servings, 2), q.servings)
  }
  return lastAmountOf(food, t, f)
}
