import type { Meal } from '@/api/types'
import { itemName } from '@/components/day/meal-entries'
import type { Messages } from '@/i18n/en'
import { clockAt } from '@/lib/zoned'

export type Slot = 'breakfast' | 'lunch' | 'snack' | 'dinner'

// slotAt is the meal of the day an instant falls in, on the user's clock:
// breakfast before 11, lunch before 15, a snack before 18, and dinner
// from then into the night.
export function slotAt(instant: number, timeZone: string): Slot {
  const hour = Number(clockAt(instant, timeZone).slice(0, 2))
  if (hour >= 4 && hour < 11) {
    return 'breakfast'
  }
  if (hour >= 11 && hour < 15) {
    return 'lunch'
  }
  if (hour >= 15 && hour < 18) {
    return 'snack'
  }
  return 'dinner'
}

// The longest description a meal's row takes for its name. A longer one,
// a model's list of what was eaten, goes under the name, which is then
// what was in it.
const nameLength = 40

// longName tells a description too long for a meal's name.
export function longName(description: string): boolean {
  return description.length > nameLength
}

// namedBy is the name of a meal of what these names name, when it has
// none of its own: the one ("Skyr"), both ("Skyr and banana"), or the
// first and how many more ("Skyr + 2"). Not the meal of the day it was
// eaten at: a day of three snacks would have three meals of one name, and
// its time says when it was.
export function namedBy(names: readonly string[], t: Messages): string {
  const [first = '', second] = names
  if (second === undefined) {
    return first
  }
  return names.length === 2 ? t.meal.both(first, second) : t.meal.andMore(first, names.length - 1)
}

// mealName is a meal's name in a row or a toast: its description, when it
// has a short one, or else what was in it (namedBy), each item by its
// name without its brand.
export function mealName(meal: Pick<Meal, 'description' | 'items'>, t: Messages): string {
  return meal.description && !longName(meal.description)
    ? meal.description
    : namedBy(
        meal.items.map((item) => itemName({ name: item.name }, t)),
        t,
      )
}
