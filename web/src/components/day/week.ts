import { addDays } from '@/i18n/format'

// weekOf is the week a day shows (WeekStrip): the seven days up to today,
// or up to the day when it is before them. The day's history is asked of
// them, once for every part of the day that reads it.
export function weekOf(date: string, today: string): { from: string; to: string } {
  const to = date < addDays(today, -6) ? date : today
  return { from: addDays(to, -6), to }
}
