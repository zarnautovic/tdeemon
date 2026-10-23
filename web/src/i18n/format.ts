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

// isDate tells a real calendar date, YYYY-MM-DD, from one of its shape
// only (2026-02-31).
export function isDate(s: string): boolean {
  return /^\d{4}-\d{2}-\d{2}$/.test(s) && addDays(s, 0) === s
}

// formatters keeps each Intl formatter once made, by its locale and
// options: a day of many items formats hundreds of numbers.
const formatters = new Map<string, Intl.NumberFormat | Intl.DateTimeFormat>()

function numberFormat(locale: string, options: Intl.NumberFormatOptions): Intl.NumberFormat {
  const key = `n|${locale}|${JSON.stringify(options)}`
  let f = formatters.get(key)
  if (!(f instanceof Intl.NumberFormat)) {
    f = new Intl.NumberFormat(locale, options)
    formatters.set(key, f)
  }
  return f
}

function dateFormat(locale: string, options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = `d|${locale}|${JSON.stringify(options)}`
  let f = formatters.get(key)
  if (!(f instanceof Intl.DateTimeFormat)) {
    f = new Intl.DateTimeFormat(locale, options)
    formatters.set(key, f)
  }
  return f
}

// parseNumber reads a number a person typed in locale's language, or
// undefined for anything else. A comma or a full stop before digits is
// the decimal point, whichever the keyboard gave, except that groups of
// three digits after the language's group separator are thousands, which
// the language's decimal point may follow: 8,042 is 8042 in English and
// 8.042 in Croatian, 1,834.5 and 1.834,5 are 1834.5, 72,5 and 72.5 are
// 72.5 in both. A number of thousands starts with no 0: 0.125 and 0,125
// are 0.125.
export function parseNumber(s: string, locale: string): number | undefined {
  const typed = s.replaceAll(/\s/g, '')
  const parts = numberFormat(locale, {}).formatToParts(1_000_000.5)
  const group = parts.find((p) => p.type === 'group')?.value
  const decimal = parts.find((p) => p.type === 'decimal')?.value ?? '.'
  const grouped =
    group !== undefined &&
    new RegExp(`^[1-9]\\d{0,2}(\\${group}\\d{3})+(\\${decimal}\\d+)?$`).test(typed)
  if (grouped) {
    return Number(typed.replaceAll(group, '').replace(decimal, '.'))
  }
  return /^\d+([.,]\d+)?$/.test(typed) ? Number(typed.replace(',', '.')) : undefined
}

// useFormat formats numbers, dates and times in the app's language.
export function useFormat() {
  const locale = useLocale()
  return {
    // number rounds to at most digits decimals.
    number: (n: number, digits = 0) =>
      numberFormat(locale, { maximumFractionDigits: digits }).format(n),
    // field is a number as a field starts with it: every decimal, near
    // enough (digits), and no group separators, which typing around
    // would break.
    field: (n: number, digits = 3) =>
      numberFormat(locale, { maximumFractionDigits: digits, useGrouping: false }).format(n),
    // parse reads a number typed in the language (parseNumber).
    parse: (s: string) => parseNumber(s, locale),
    // percent is a percentage, 18.2 for 18.2 %, as the language writes it.
    percent: (n: number, digits = 1) =>
      numberFormat(locale, { style: 'percent', maximumFractionDigits: digits }).format(n / 100),
    // date is a day's date in words: "Wednesday, October 22" in English,
    // "srijeda, 22. listopada" in Croatian.
    date: (date: string) =>
      dateFormat(locale, {
        weekday: 'long',
        day: 'numeric',
        month: 'long',
        timeZone: 'UTC',
      }).format(calendarDate(date)),
    // time is an instant's time of day on the user's clock.
    time: (instant: string, timeZone: string) =>
      dateFormat(locale, { hour: 'numeric', minute: '2-digit', timeZone }).format(
        new Date(instant),
      ),
    // day is an instant's day on the user's clock, short: "Oct 21" in
    // English, "21. lis" in Croatian; of another year than now's, with
    // it.
    day: (instant: string, timeZone: string, now: number = Date.now()) => {
      const year = (at: Date) => dateFormat(locale, { year: 'numeric', timeZone }).format(at)
      const at = new Date(instant)
      return dateFormat(locale, {
        day: 'numeric',
        month: 'short',
        ...(year(at) === year(new Date(now)) ? {} : { year: 'numeric' }),
        timeZone,
      }).format(at)
    },
  }
}
