import type { components } from '@/api/schema'
import type { Food, Item, Meal } from '@/api/types'
import { uuidv7 } from '@/lib/uuid'

type ItemInput = components['schemas']['ItemInput']
type LogMealBody = components['schemas']['LogMealBody']

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
  // Servings without their name, of the food's one: its amount in its
  // unit, as the item had it.
  const last = food.last_use
  if (
    q?.servings !== undefined &&
    q.serving === undefined &&
    last !== undefined &&
    units.includes(last.unit)
  ) {
    return { key, kind: 'food', food, amount: field(last.amount), unit: last.unit }
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

// nameKey is a name as the server tells names apart (food.Key): in one
// form, case folded, and its spaces one. JavaScript has no case folding
// of its own: lower case, with the letters it folds otherwise (ß and ẞ
// are ss, a final sigma is a sigma).
function nameKey(name: string): string {
  return name
    .normalize('NFC')
    .toLowerCase()
    .replaceAll('ß', 'ss')
    .replaceAll('ς', 'σ')
    .split(/\s+/)
    .filter(Boolean)
    .join(' ')
}

// servingNamed is a food's serving the name names, as the server finds
// it: Piece is its piece.
function servingNamed(food: Food, name: string) {
  return food.servings?.find((s) => nameKey(s.name) === nameKey(name))
}

// unitIn is unit as the food names it among its units (unitsOf): a
// serving given as Piece is its serving:piece.
export function unitIn(food: Food, unit: string): string {
  const serving = servingOf(unit)
  const named = serving === undefined ? undefined : servingNamed(food, serving)
  return named === undefined ? unit : servingUnit + named.name
}

// unitGrams is how many grams (or millilitres) one of a food's unit is,
// or undefined when the food does not say: a portion without its grams.
export function unitGrams(food: Food, unit: string): number | undefined {
  const serving = servingOf(unit)
  if (serving !== undefined) {
    return servingNamed(food, serving)?.amount
  }
  return unit === 'portion' ? food.portion_grams : 1
}

// quantityIn is n of a food in unit (unitsOf), as an item gives it.
export function quantityIn(unit: string, n: number): ItemInput {
  const serving = servingOf(unit)
  if (serving !== undefined) {
    return { serving, servings: n }
  }
  if (unit === 'portion') {
    return { portions: n }
  }
  return { amount: n, unit: unit === 'ml' ? 'ml' : 'g' }
}

// itemOf is the item an entry sends, with parse reading its numbers; or
// undefined when one is not a number, a food's amount is 0, or the
// estimate has no name.
export function itemOf(
  entry: Entry,
  parse: (s: string) => number | undefined,
): ItemInput | undefined {
  if (entry.kind === 'food') {
    const n = parse(entry.amount)
    if (n === undefined || n <= 0) {
      return undefined
    }
    return { id: entry.key, food_id: entry.food.id, ...quantityIn(entry.unit, n) }
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

// An amount is how much of an item was given, as it was given (grams,
// portions, servings), and the item that puts another number in its
// place, given the same way: the item's food (as it is when it is sent)
// worked out again from the food as it is now, so a food corrected since
// gives its new values. Only an item of a food has one: an estimate is of
// a whole plate. Its unit is a serving's name or the food's unit, or, in
// portions, the word for as many as are typed; its key, the unit as
// unitsOf names it.
export interface Amount {
  n: number
  unit: string | undefined
  key: string
  portions?: true
  with: (item: Item & { food_id: string }, n: number) => ItemInput
}

// foodItem is an item's food and note, without how much.
export function foodItem(item: Item & { food_id: string }): ItemInput {
  return item.note === undefined
    ? { food_id: item.food_id }
    : { food_id: item.food_id, note: item.note }
}

export function amountOf(item: Item): Amount | undefined {
  if (item.food_id === undefined) {
    return undefined
  }
  const q = item.quantity
  if (q.servings !== undefined && q.serving !== undefined) {
    const serving = q.serving
    return {
      n: q.servings,
      unit: serving,
      key: servingUnit + serving,
      with: (it, n) => ({ ...foodItem(it), serving, servings: n }),
    }
  }
  // Servings without their name, of the food's one: in its unit, g or ml.
  if (q.servings !== undefined && item.unit !== 'portion') {
    const unit = item.unit
    return {
      n: item.amount,
      unit,
      with: (it, n) => ({ ...foodItem(it), amount: n, unit }),
    }
  }
  if (q.portions !== undefined) {
    return {
      n: q.portions,
      unit: undefined,
      key: 'portion',
      portions: true,
      with: (it, n) => ({ ...foodItem(it), portions: n }),
    }
  }
  if (q.amount !== undefined) {
    // Given without a unit, it is the food's, and stays so.
    const given = q.unit
    return {
      n: q.amount,
      unit: given ?? (item.unit === 'portion' ? undefined : item.unit),
      key: given ?? item.unit,
      with: (it, n) =>
        given === undefined
          ? { ...foodItem(it), amount: n }
          : { ...foodItem(it), amount: n, unit: given },
    }
  }
  return undefined
}

// inputOf is an item as it is logged again: a food's in the amount it
// was given, as the food is now, or an estimate with its values; each
// with its note.
export function inputOf(item: Item): ItemInput {
  const note = item.note === undefined ? {} : { note: item.note }
  if (item.food_id === undefined) {
    return { name: item.name, nutrients: item.nutrients, ...note }
  }
  const food = { ...item, food_id: item.food_id }
  const amount = amountOf(food)
  if (amount !== undefined) {
    return amount.with(food, amount.n)
  }
  // Given without a quantity, as an import may have: its amount in the
  // food's unit.
  return item.unit === 'portion'
    ? { food_id: item.food_id, portions: item.amount, ...note }
    : { food_id: item.food_id, amount: item.amount, unit: item.unit, ...note }
}

// repeatOf is a meal logged again, today at this time: its items as
// inputOf has them, its description and its note, under an id made now,
// so each repeat is a meal of its own.
export function repeatOf(meal: Meal): LogMealBody {
  return {
    id: uuidv7(),
    ...(meal.description === '' ? {} : { description: meal.description }),
    ...(meal.note === undefined || meal.note === '' ? {} : { note: meal.note }),
    items: meal.items.map(inputOf),
  }
}
