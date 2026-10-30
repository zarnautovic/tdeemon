import type { components } from '@/api/schema'
import type { Food, Item, Meal, Nutrients } from '@/api/types'
import {
  portionServing,
  recipeQuantity,
  shareField,
  wholeServing,
} from '@/components/recipes/share'
import type { Messages } from '@/i18n/en'
import type { useFormat } from '@/i18n/format'
import limits from '@/lib/limits.json'
import { nameKey } from '@/lib/name-key'
import { uuidv7 } from '@/lib/uuid'

type ItemInput = components['schemas']['ItemInput']
type LogMealBody = components['schemas']['LogMealBody']
type Format = ReturnType<typeof useFormat>

// An entry is an item of a meal being logged, as typed: a food of the
// user's in an amount and a unit of it, or an estimate of a whole plate.
// Its key is the item's id too, so a meal sent again is the same. A
// food's exact is the amount a unit chosen worked out, before it was
// rounded to be shown, until the amount is typed: it is what is sent.
export type Entry =
  | { key: string; kind: 'food'; food: Food; amount: string; unit: string; exact?: number }
  | {
      key: string
      kind: 'estimate'
      name: string
      kcal: string
      protein: string
      carbs: string
      fat: string
    }

// A food entry is an entry of a food of the user's.
export type FoodEntry = Entry & { kind: 'food' }

// A unit of a food's: its grams or millilitres (g, ml), its portion
// (portion), or a serving it names (serving:<name>).
const servingUnit = 'serving:'

// unitsOf are the units a food is given in: its basis's, the grams of a
// portion when it has them, and its servings. A recipe's are its portion
// (when divided), a share of the whole dish and grams, in that order: a
// recipe is eaten by the portion or the share, grams the last resort.
export function unitsOf(food: Food): string[] {
  const base = food.basis === '100ml' ? ['ml'] : food.basis === '100g' ? ['g'] : ['portion']
  if (food.basis === 'portion' && food.portion_grams !== undefined) {
    base.push('g')
  }
  const units = [...base, ...(food.servings ?? []).map((s) => servingUnit + s.name)]
  if (food.source !== 'recipe') {
    return units
  }
  const first = [servingUnit + portionServing, servingUnit + wholeServing, ...base]
  return [...first.filter((u) => units.includes(u)), ...units.filter((u) => !first.includes(u))]
}

// parseAmount reads an amount typed: a number as parse reads it, a
// fraction of two (1/10, 2/3, 1,5/4), or a whole number and a fraction (1
// 1/2), as a share of a recipe is written; undefined for anything else,
// and for a fraction of 0 below it.
export function parseAmount(
  s: string,
  parse: (s: string) => number | undefined,
): number | undefined {
  const typed = s.trim()
  const mixed = /^(\d+)\s+(\d+)\s*\/\s*(\d+)$/.exec(typed)
  if (mixed) {
    const [, whole = '', n = '', d = ''] = mixed
    return Number(d) > 0 ? Number(whole) + Number(n) / Number(d) : undefined
  }
  const fraction = /^([^/]+)\/([^/]+)$/.exec(typed)
  if (fraction) {
    const n = parse(fraction[1] ?? '')
    const d = parse(fraction[2] ?? '')
    return n === undefined || d === undefined || d === 0 ? undefined : n / d
  }
  return parse(typed)
}

// entryAmount is how much of its food an entry has: as a unit chosen
// worked it out, else as typed (parseAmount); undefined while it is not
// a number.
export function entryAmount(
  entry: { amount: string; exact?: number | undefined },
  parse: (s: string) => number | undefined,
): number | undefined {
  return entry.exact ?? parseAmount(entry.amount, parse)
}

// isShare tells the unit of a share of a recipe's whole dish.
export function isShare(food: { source?: string | undefined }, unit: string): boolean {
  return food.source === 'recipe' && unit === servingUnit + wholeServing
}

// servingOf is the serving a unit names, or undefined for another unit.
export function servingOf(unit: string): string | undefined {
  return unit.startsWith(servingUnit) ? unit.slice(servingUnit.length) : undefined
}

// foodEntry is a food added to the meal, in the amount it was eaten in
// last, or else 100 of its unit, or one portion; a recipe, one of its
// portions, or a quarter of it when it is not divided. A share of a
// recipe is typed as a fraction (shareField), and keeps every decimal it
// had: a third is a third, not 0.333.
export function foodEntry(food: Food, field: (n: number, digits?: number) => string): FoodEntry {
  const key = uuidv7()
  const q = food.last_use?.quantity
  const units = unitsOf(food)
  if (
    q?.servings !== undefined &&
    q.serving !== undefined &&
    units.includes(servingUnit + q.serving)
  ) {
    const unit = servingUnit + q.serving
    if (isShare(food, unit)) {
      return {
        key,
        kind: 'food',
        food,
        amount: shareField(q.servings, field),
        unit,
        exact: q.servings,
      }
    }
    return { key, kind: 'food', food, amount: field(q.servings), unit }
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
  if (food.source === 'recipe' && first !== 'g') {
    return {
      key,
      kind: 'food',
      food,
      amount: isShare(food, first) ? '1/4' : field(1),
      unit: first,
    }
  }
  return { key, kind: 'food', food, amount: first === 'portion' ? '1' : '100', unit: first }
}

// estimateEntry is a plate given by its values.
export function estimateEntry(): Entry {
  return { key: uuidv7(), kind: 'estimate', name: '', kcal: '', protein: '', carbs: '', fat: '' }
}

// servingNamed is a food's serving the name names, as the server finds
// it: of that name, or else the one of its key (Piece is its piece); none
// when the key is of two, which the server's folding never makes.
function servingNamed(food: Food, name: string) {
  const servings = food.servings ?? []
  const named = servings.find((s) => s.name === name)
  if (named !== undefined) {
    return named
  }
  const keyed = servings.filter((s) => nameKey(s.name) === nameKey(name))
  return keyed.length === 1 ? keyed[0] : undefined
}

// unitIn is unit as the food names it among its units (unitsOf): a
// serving given as Piece is its serving:piece.
export function unitIn(food: Food, unit: string): string {
  const serving = servingOf(unit)
  const named = serving === undefined ? undefined : servingNamed(food, serving)
  return named === undefined ? unit : servingUnit + named.name
}

// unitGrams is how many of the food's own unit, grams or millilitres,
// one of unit is; or undefined when the food does not say: a portion
// without its grams, or grams of a food in millilitres (or the other way
// round, an item logged before its basis was corrected), of no density.
export function unitGrams(food: Food, unit: string): number | undefined {
  const serving = servingOf(unit)
  if (serving !== undefined) {
    return servingNamed(food, serving)?.amount
  }
  if (unit === 'portion') {
    return food.portion_grams
  }
  return (unit === 'ml') === (food.basis === '100ml') ? 1 : undefined
}

// converted is n of a food in unit from worked out in unit to, or
// undefined when the food does not say the grams of both (unitGrams).
// One a hair off the amount rounded to be shown is that amount: 100 g in
// servings of 30 g and back is 100 g, not 100.00000000000001.
export function converted(food: Food, from: string, to: string, n: number): number | undefined {
  const fromGrams = unitGrams(food, from)
  const toGrams = unitGrams(food, to)
  if (!fromGrams || !toGrams) {
    return undefined
  }
  const worked = (n * fromGrams) / toGrams
  const rounded = Math.round(worked * 100) / 100
  return Math.abs(worked - rounded) <= 1e-9 * Math.max(1, rounded) ? rounded : worked
}

// Values are the four values every food and item has.
export type Values = Pick<Nutrients, 'kcal' | 'protein' | 'carbs' | 'fat'>

// nutrientsIn is what n of a food in unit (unitsOf) brings, as the server
// works it out: the food's values for 100 g or ml times the grams or ml,
// or for a portion times the portions; or undefined when the food does
// not say how much one of unit is (unitGrams), or a food by the portion
// is given in a serving, which the server refuses.
export function nutrientsIn(food: Food, unit: string, n: number): Values | undefined {
  let times: number | undefined
  if (food.basis === 'portion') {
    if (unit === 'portion') {
      times = n
    } else if (unit === 'g' && food.portion_grams) {
      times = n / food.portion_grams
    }
  } else {
    const grams = unitGrams(food, unit)
    times = grams === undefined ? undefined : (n * grams) / 100
  }
  if (times === undefined) {
    return undefined
  }
  const { kcal, protein, carbs, fat } = food.nutrients
  return { kcal: kcal * times, protein: protein * times, carbs: carbs * times, fat: fat * times }
}

// entryValues is what an entry brings, with parse reading its numbers: a
// food's in its amount (nutrientsIn), an estimate's as typed; undefined
// while one is not a number.
export function entryValues(
  entry: Entry,
  parse: (s: string) => number | undefined,
): Values | undefined {
  if (entry.kind === 'food') {
    const n = entryAmount(entry, parse)
    return n === undefined ? undefined : nutrientsIn(entry.food, entry.unit, n)
  }
  const kcal = parse(entry.kcal)
  const protein = parse(entry.protein)
  const carbs = parse(entry.carbs)
  const fat = parse(entry.fat)
  return kcal === undefined || protein === undefined || carbs === undefined || fat === undefined
    ? undefined
    : { kcal, protein, carbs, fat }
}

// shownAmount is an amount a unit chosen worked out, as its field shows
// it: rounded to two decimals, but not to none, as 1 g of a serving of
// 500 g is 0.002 of it; field formats it.
export function shownAmount(n: number, field: (n: number, digits?: number) => string): string {
  const rounded = Math.round(n * 100) / 100
  return rounded > 0 || n === 0 ? field(rounded) : field(Number(n.toPrecision(2)), 10)
}

// inUnit is a food's entry in another of its units, its amount worked
// out anew when the food says the grams of both, from the amount a unit
// chosen worked out or else the one typed; otherwise emptied, to be typed
// anew in the unit chosen rather than taken over in it.
export function inUnit(
  entry: FoodEntry,
  unit: string,
  parse: (s: string) => number | undefined,
  field: (n: number, digits?: number) => string,
): FoodEntry {
  const { exact: _was, ...rest } = entry
  const base = entryAmount(entry, parse)
  const n = base === undefined ? undefined : converted(entry.food, entry.unit, unit, base)
  if (n === undefined) {
    return { ...rest, unit, amount: '' }
  }
  // A share of a recipe as its fraction, when it is one: 300 g of 1200 g
  // are 1/4 of it.
  const shown = isShare(entry.food, unit) ? shareField(n, field) : shownAmount(n, field)
  return { ...rest, unit, amount: shown, exact: n }
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

// A Wrong is why an entry cannot be sent, as its field says it: an
// amount or a value that is not a number, an amount of 0 or less, an
// estimate without a name, or past the most an item may hold
// (limits.json, the server's) in a unit: grams, millilitres, servings,
// portions, or an estimate's kcal.
export type Wrong =
  | { kind: 'number' | 'none' | 'name' }
  | { kind: 'most'; most: number; unit: 'g' | 'ml' | 'serving' | 'portion' | 'kcal' }

// amountWrong is what is wrong with n of food in unit (unitsOf), if
// anything: 0 or less, grams or ml past the most, servings past the most
// of them or of their grams, portions past the most, as the grams of a
// food by the portion may be too. Of a food not known yet, the count
// alone.
export function amountWrong(food: Food | undefined, unit: string, n: number): Wrong | undefined {
  const { maxAmount, maxServings, maxPortions } = limits.meal
  if (n <= 0) {
    return { kind: 'none' }
  }
  if (servingOf(unit) !== undefined) {
    const grams = food && unitGrams(food, unit)
    const most = grams === undefined ? maxServings : Math.min(maxServings, maxAmount / grams)
    return n > most ? { kind: 'most', most: Math.floor(most), unit: 'serving' } : undefined
  }
  if (unit === 'portion') {
    return n > maxPortions ? { kind: 'most', most: maxPortions, unit: 'portion' } : undefined
  }
  const own = unit === 'ml' ? 'ml' : 'g'
  if (n > maxAmount) {
    return { kind: 'most', most: maxAmount, unit: own }
  }
  if (food?.basis === 'portion' && food.portion_grams !== undefined) {
    const most = maxPortions * food.portion_grams
    return n > most ? { kind: 'most', most, unit: own } : undefined
  }
  return undefined
}

// entryWrong is what is wrong with an entry, with parse reading its
// numbers, if anything: a food's amount (amountWrong), or an estimate
// without a name, with a value that is not a number, or with more energy
// than one may have.
export function entryWrong(
  entry: Entry,
  parse: (s: string) => number | undefined,
): Wrong | undefined {
  if (entry.kind === 'food') {
    const n = entryAmount(entry, parse)
    return n === undefined ? { kind: 'number' } : amountWrong(entry.food, entry.unit, n)
  }
  if (entry.name.trim() === '') {
    return { kind: 'name' }
  }
  const kcal = parse(entry.kcal)
  if (
    kcal === undefined ||
    [entry.protein, entry.carbs, entry.fat].some((v) => parse(v) === undefined)
  ) {
    return { kind: 'number' }
  }
  const most = limits.meal.maxEstimateKcal
  return kcal > most ? { kind: 'most', most, unit: 'kcal' } : undefined
}

// sayWrong is what w says at its field, its numbers as number says them.
export function sayWrong(w: Wrong, t: Messages, number: (n: number) => string): string {
  return w.kind === 'most' ? t.log.wrong.most[w.unit](number(w.most), w.most) : t.log.wrong[w.kind]
}

// entryItem is the item an entry sends, with parse reading its numbers,
// or what is wrong with it (entryWrong).
export function entryItem(
  entry: Entry,
  parse: (s: string) => number | undefined,
): { item: ItemInput } | { wrong: Wrong } {
  const wrong = entryWrong(entry, parse)
  if (wrong !== undefined) {
    return { wrong }
  }
  if (entry.kind === 'food') {
    const n = entryAmount(entry, parse)
    return n === undefined
      ? { wrong: { kind: 'number' } }
      : { item: { id: entry.key, food_id: entry.food.id, ...quantityIn(entry.unit, n) } }
  }
  const nutrients = entryValues(entry, parse)
  return nutrients === undefined
    ? { wrong: { kind: 'number' } }
    : { item: { id: entry.key, name: entry.name.trim(), nutrients } }
}

// itemOf is the item an entry sends, or undefined when something is
// wrong with it (entryItem).
export function itemOf(
  entry: Entry,
  parse: (s: string) => number | undefined,
): ItemInput | undefined {
  const checked = entryItem(entry, parse)
  return 'item' in checked ? checked.item : undefined
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
      key: unit,
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

// servingsOf is servings of a food's serving, n as shown: "2 slice", "1
// slice". A serving whose name holds a number of its own ("½ cup") is
// counted with ×, and one of it is the name alone, as "1 ½ cup" reads as
// one and a half.
export function servingsOf(n: string, servings: number, serving: string): string {
  if (!/^[\d½⅓⅔¼¾⅕⅙⅛]/u.test(serving)) {
    return `${n} ${serving}`
  }
  return servings === 1 ? serving : `${n} × ${serving}`
}

// lastAmountOf is how much of a food was eaten last, as it was given: "2
// slice", "1.5 portions", "200 g", of a recipe "⅒ of the recipe" or "1
// portion" (recipeQuantity); undefined for a food not eaten yet.
export function lastAmountOf(food: Food, t: Messages, f: Format): string | undefined {
  const last = food.last_use
  if (last === undefined) {
    return undefined
  }
  const ofRecipe = recipeQuantity(food.source, last.quantity, t, f.number)
  if (ofRecipe !== undefined) {
    return ofRecipe
  }
  const { servings, serving } = last.quantity
  if (servings !== undefined && serving !== undefined) {
    return servingsOf(f.number(servings, 2), servings, serving)
  }
  return last.unit === 'portion'
    ? t.day.portions(f.number(last.amount, 2), last.amount)
    : t.unit[last.unit](f.number(last.amount))
}

// splitRest is the name the one-off import of the maintainer's history
// gave the item of the rest of a meal's total (DESIGN.md §10), which the
// meals of before still hold.
const splitRest = 'Rest of the meal (history split)'

// itemName is an item's name as the app shows it: the one the import of
// history gives, in the language; and after it, as the foods are shown,
// its food's brand, when it has one ("Tuna in its own juice · Eva").
export function itemName(item: Pick<Item, 'name' | 'brand'>, t: Messages): string {
  const name = item.name === splitRest ? t.meal.restOfMeal : item.name
  return item.brand ? `${name} · ${item.brand}` : name
}
