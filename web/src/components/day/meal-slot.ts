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
