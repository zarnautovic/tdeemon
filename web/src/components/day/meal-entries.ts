import type { components } from '@/api/schema'
import type { Food } from '@/api/types'
import { uuidv7 } from '@/lib/uuid'

type ItemInput = components['schemas']['ItemInput']

// An entry is an item of a meal being logged, as typed: a food of the
// user's in an amount and a unit of it, or an estimate of a whole plate.
// Its key is the item's id too, so a meal sent again is the same.
export type Entry =
  | { key: string; kind: 'food'; food: Food; amount: string; unit: string }
  | {
      key: string
      kind: 'estimate'
      name: string
      kcal: string
      protein: string
      carbs: string
      fat: string
    }

// A unit of a food's: its grams or millilitres (g, ml), its portion
// (portion), or a serving it names (serving:<name>).
const servingUnit = 'serving:'

// unitsOf are the units a food is given in: its basis's, the grams of a
// portion when it has them, and its servings.
export function unitsOf(food: Food): string[] {
  const base = food.basis === '100ml' ? ['ml'] : food.basis === '100g' ? ['g'] : ['portion']
  if (food.basis === 'portion' && food.portion_grams !== undefined) {
    base.push('g')
  }
  return [...base, ...(food.servings ?? []).map((s) => servingUnit + s.name)]
}

// servingOf is the serving a unit names, or undefined for another unit.
export function servingOf(unit: string): string | undefined {
  return unit.startsWith(servingUnit) ? unit.slice(servingUnit.length) : undefined
}

// foodEntry is a food added to the meal, in the amount it was eaten in
// last, or else 100 of its unit, or one portion.
export function foodEntry(food: Food, field: (n: number) => string): Entry {
  const key = uuidv7()
  const q = food.last_use?.quantity
  const units = unitsOf(food)
  if (
    q?.servings !== undefined &&
    q.serving !== undefined &&
    units.includes(servingUnit + q.serving)
  ) {
    return { key, kind: 'food', food, amount: field(q.servings), unit: servingUnit + q.serving }
  }
  if (q?.portions !== undefined && units.includes('portion')) {
    return { key, kind: 'food', food, amount: field(q.portions), unit: 'portion' }
  }
  const unit = q?.unit ?? food.last_use?.unit
  if (q?.amount !== undefined && unit !== undefined && units.includes(unit)) {
    return { key, kind: 'food', food, amount: field(q.amount), unit }
  }
  const first = units[0] ?? 'g'
  return { key, kind: 'food', food, amount: first === 'portion' ? '1' : '100', unit: first }
}

// estimateEntry is a plate given by its values.
export function estimateEntry(): Entry {
  return { key: uuidv7(), kind: 'estimate', name: '', kcal: '', protein: '', carbs: '', fat: '' }
}

// itemOf is the item an entry sends, with parse reading its numbers; or
// undefined when one is not a number, or the estimate has no name.
export function itemOf(
  entry: Entry,
  parse: (s: string) => number | undefined,
): ItemInput | undefined {
  if (entry.kind === 'food') {
    const n = parse(entry.amount)
    if (n === undefined) {
      return undefined
    }
    const base = { id: entry.key, food_id: entry.food.id }
    const serving = servingOf(entry.unit)
    if (serving !== undefined) {
      return { ...base, serving, servings: n }
    }
    if (entry.unit === 'portion') {
      return { ...base, portions: n }
    }
    return { ...base, amount: n, unit: entry.unit === 'ml' ? 'ml' : 'g' }
  }
  const values = [entry.kcal, entry.protein, entry.carbs, entry.fat].map(parse)
  const [kcal, protein, carbs, fat] = values
  const name = entry.name.trim()
  if (
    name === '' ||
    kcal === undefined ||
    protein === undefined ||
    carbs === undefined ||
    fat === undefined
  ) {
    return undefined
  }
  return { id: entry.key, name, nutrients: { kcal, protein, carbs, fat } }
}
