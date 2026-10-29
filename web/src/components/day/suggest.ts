import type { Eaten, Food, RecentMeal } from '@/api/types'
import { slotAt, type Slot } from '@/components/day/meal-slot'

// How far back what was eaten at a meal of the day counts: two weeks.
const counted = 14 * 24 * 60 * 60 * 1000

// A moment is what the quick add suggests by: the meal of the day being
// logged (slot), on the user's clock; now, back from which the last two
// weeks count; and the day it is logged on, whose meals are had already.
export interface Moment {
  slot: Slot
  now: number
  timeZone: string
  // YYYY-MM-DD.
  date: string
}

// timesAt is how many of the times eaten in the last two weeks were at
// the moment's meal of the day.
function timesAt(eaten: readonly Eaten[], at: Moment): number {
  return eaten.filter((e) => {
    const instant = Date.parse(e.at)
    return (
      instant <= at.now && instant > at.now - counted && slotAt(instant, at.timeZone) === at.slot
    )
  }).length
}

// runOf is the run of ordered a thing is in: 0 eaten at the meal of the
// day lately, 1 not, 2 had by the day already.
function runOf(thing: { times: number; had: boolean }): number {
  return thing.had ? 2 : thing.times > 0 ? 0 : 1
}

// ordered is things in three runs (runOf), each in the order given
// otherwise: those eaten at the meal of the day lately (times), the most
// often first; the others; and last those the day has had already, as
// what was just logged is not what comes next.
function ordered<T>(things: readonly T[], times: (t: T) => number, had: (t: T) => boolean): T[] {
  return things
    .map((thing, i) => ({ thing, i, times: times(thing), had: had(thing) }))
    .toSorted(
      (a, b) => runOf(a) - runOf(b) || (runOf(a) === 0 ? b.times - a.times : 0) || a.i - b.i,
    )
    .map((s) => s.thing)
}

// suggestMeals are meals of before (the latest first), as the quick add
// offers them at a moment: those eaten at its meal of the day in the last
// two weeks, the most often first, then the others, then those the day
// had; and how many of the first are of the meal of the day (ofSlot).
export function suggestMeals(
  meals: readonly RecentMeal[],
  at: Moment,
): { meals: RecentMeal[]; ofSlot: number } {
  const had = (m: RecentMeal) => m.eaten.some((e) => e.day === at.date)
  return {
    meals: ordered(meals, (m) => timesAt(m.eaten, at), had),
    ofSlot: meals.filter((m) => !had(m) && timesAt(m.eaten, at) > 0).length,
  }
}

// suggestFoods are foods in the order given, as the quick add offers
// them at a moment, as it does the meals of before: those the meals of
// before had at its meal of the day in the last two weeks, the most often
// first (yoghurt in the morning), then the others, then those the day had
// already, by its meals of before or by their last use.
export function suggestFoods(
  foods: readonly Food[],
  meals: readonly RecentMeal[],
  at: Moment,
): Food[] {
  const times = new Map<string, number>()
  const had = new Set<string>()
  for (const m of meals) {
    const n = timesAt(m.eaten, at)
    const today = m.eaten.some((e) => e.day === at.date)
    for (const id of new Set(
      m.items.flatMap((i) => (i.food_id === undefined ? [] : [i.food_id])),
    )) {
      times.set(id, (times.get(id) ?? 0) + n)
      if (today) {
        had.add(id)
      }
    }
  }
  return ordered(
    foods,
    (food) => times.get(food.id) ?? 0,
    (food) => had.has(food.id) || food.last_use?.day === at.date,
  )
}
