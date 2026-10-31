import type { components } from '@/api/schema'

export type Weekday = components['schemas']['AutopilotBody']['weekday']

// week is the days of the week as the API names them, Monday first, as
// the app's weeks are.
export const week: readonly Weekday[] = [
  'monday',
  'tuesday',
  'wednesday',
  'thursday',
  'friday',
  'saturday',
  'sunday',
]

// weekdayOf is the weekday of a day's date (YYYY-MM-DD), as the API
// names it: a review's next date is on its weekday.
export function weekdayOf(date: string): Weekday {
  // getUTCDay counts from Sunday, 0; week from Monday.
  const day = new Date(`${date}T00:00:00Z`).getUTCDay()
  return week[(day + 6) % 7] ?? 'monday'
}
