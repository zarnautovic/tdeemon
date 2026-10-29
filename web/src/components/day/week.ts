import { addDays } from '@/i18n/format'
import limits from '@/lib/limits.json'

// weekOf is the week a day shows (WeekStrip): the seven days up to today,
// or up to the day when it is before them; near the first day a history
// is of (limits.json), the seven days from it, as no history goes further
// back. The day's history is asked of them, once for every part of the
// day that reads it.
export function weekOf(date: string, today: string): { from: string; to: string } {
  const to = date < addDays(today, -6) ? date : today
  const from = addDays(to, -6)
  const first = limits.history.since
  return from < first ? { from: first, to: addDays(first, 6) } : { from, to }
}
