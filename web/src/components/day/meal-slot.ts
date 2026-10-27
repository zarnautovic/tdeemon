import type { Meal } from '@/api/types'
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
// the meal of the day it was eaten at (Lunch).
const nameLength = 40

// longName tells a description too long for a meal's name.
export function longName(description: string): boolean {
  return description.length > nameLength
}

// mealName is a meal's name in a row: its description, when it has a
// short one, or else the meal of the day it was eaten at, in words.
export function mealName(
  meal: Pick<Meal, 'description' | 'eaten_at'>,
  timeZone: string,
  slots: Record<Slot, string>,
): string {
  return meal.description && !longName(meal.description)
    ? meal.description
    : slots[slotAt(Date.parse(meal.eaten_at), timeZone)]
}
