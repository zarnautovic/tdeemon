import type { components } from '@/api/schema'

type Nutrients = components['schemas']['FoodNutrientsBody']

// kcalOf is the energy a food's values bring, by the factors of an EU
// label (Regulation 1169/2011, Annex XIV): 4 kcal a gram of protein and
// of carbs, 9 of fat, 7 of alcohol, 2.4 of polyols and 2 of fibre. The
// carbs hold the polyols, as on an EU label; the fibre is apart.
export function kcalOf(n: Omit<Nutrients, 'kcal'>): number {
  const polyols = Math.min(n.polyols ?? 0, n.carbs)
  return (
    4 * n.protein +
    4 * (n.carbs - polyols) +
    2.4 * polyols +
    9 * n.fat +
    7 * (n.alcohol ?? 0) +
    2 * (n.fiber ?? 0)
  )
}

// How far the energy a food says may be from what its values bring, as
// a label's rounding and a food's own makeup take it: a fifth of the
// larger, and at least 15 kcal.
const share = 0.2
const least = 15

// energyOff is the energy the values of n bring, when the energy n says
// is further from it than a label would be; undefined when the two
// agree. A food's form warns of it, and does not refuse it.
export function energyOff(n: Nutrients): number | undefined {
  const made = kcalOf(n)
  const off = Math.abs(n.kcal - made)
  return off > least && off > share * Math.max(n.kcal, made) ? made : undefined
}
