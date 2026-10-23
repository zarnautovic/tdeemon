import { useLocale } from './locale'

// calendarDate is a day's date, YYYY-MM-DD, as an instant read and shown
// in UTC, so no time zone moves it to the day before or after.
function calendarDate(date: string): Date {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date)
  if (!m) {
    throw new RangeError(`not a date: ${date}`)
  }
  return new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])))
}

// addDays is the date n days after date (before, for n below 0).
export function addDays(date: string, n: number): string {
  const d = calendarDate(date)
  d.setUTCDate(d.getUTCDate() + n)
  return d.toISOString().slice(0, 10)
}

// useFormat formats numbers, dates and times in the app's language.
export function useFormat() {
  const locale = useLocale()
  return {
    // number rounds to at most digits decimals.
    number: (n: number, digits = 0) =>
      new Intl.NumberFormat(locale, { maximumFractionDigits: digits }).format(n),
    // date is a day's date in words: "Wednesday, 22 October".
    date: (date: string) =>
      new Intl.DateTimeFormat(locale, {
        weekday: 'long',
        day: 'numeric',
        month: 'long',
        timeZone: 'UTC',
      }).format(calendarDate(date)),
    // time is an instant's time of day on the user's clock.
    time: (instant: string, timeZone: string) =>
      new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit', timeZone }).format(
        new Date(instant),
      ),
  }
}
