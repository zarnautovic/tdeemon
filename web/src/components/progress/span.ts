import { addDays, daysBetween } from '@/i18n/format'

// Span is the period the progress shows: so many days up to today, or the
// phase today is in.
export type Span = '30' | '90' | '180' | '365' | 'phase'

export const spans: readonly Span[] = ['30', '90', '180', '365', 'phase']

// A history is of 366 days at most (the API's), and its table of weeks
// of a year's.
const mostDays = 366
const mostWeeks = 52

// firstDay is a span's first day, up to today: the phase's first, but no
// more than the days a history has.
export function firstDay(span: Span, today: string, phaseStart?: string): string {
  if (span !== 'phase') {
    return addDays(today, 1 - Number(span))
  }
  const earliest = addDays(today, 1 - mostDays)
  return phaseStart === undefined || phaseStart < earliest ? earliest : phaseStart
}

// byMonth reports whether a span's axis names months rather than days:
// one of more than half a year.
export function byMonth(from: string, today: string): boolean {
  return daysBetween(from, today) + 1 > 180
}

// weekStart is the Monday of a date's week: a calendar week runs from
// Monday to Sunday, as the server's (day.Date.WeekStart).
export function weekStart(date: string): string {
  const weekday = new Date(`${date}T00:00:00Z`).getUTCDay()
  return addDays(date, -((weekday + 6) % 7))
}

// weeksOf is the table of weeks of a span: the calendar weeks with a day
// of it up to yesterday (today is open), the last up to yesterday, at
// most a year's. from is the history to ask for, the Monday a week before
// the first, whose trend weight the first's change is measured from;
// count the weeks shown, 0 when the span has none over yet.
export function weeksOf(first: string, today: string): { from: string; to: string; count: number } {
  const to = addDays(today, -1)
  const last = weekStart(to)
  const weeks = to < first ? 0 : daysBetween(weekStart(first), last) / 7 + 1
  const count = Math.max(0, Math.min(mostWeeks, weeks))
  return { from: addDays(last, -7 * count), to, count }
}
