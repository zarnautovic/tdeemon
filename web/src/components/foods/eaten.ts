import type { Food } from '@/api/types'
import { daysBetween } from '@/i18n/format'

// daysSinceEaten is how many days before today a food was eaten last:
// undefined when it never was, or while today is not known.
export function daysSinceEaten(food: Food, today: string | undefined): number | undefined {
  return today === undefined || food.last_use === undefined
    ? undefined
    : daysBetween(food.last_use.day, today)
}

// eatenThisWeek tells a food eaten in the last 7 days, today's among them.
export function eatenThisWeek(food: Food, today: string | undefined): boolean {
  const days = daysSinceEaten(food, today)
  return days !== undefined && days < 7
}
