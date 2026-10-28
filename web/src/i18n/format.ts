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

// daysBetween is how many days to is after from (before, below 0).
export function daysBetween(from: string, to: string): number {
  return Math.round((calendarDate(to).getTime() - calendarDate(from).getTime()) / 86_400_000)
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

// shortDay is at's day on the clock of timeZone, short, with its year
// when another than now's on the clock of nowZone.
function shortDay(
  locale: string,
  at: Date,
  timeZone: string,
  now: number,
  nowZone: string = timeZone,
): string {
  const year = (d: Date, zone: string) =>
    dateFormat(locale, { year: 'numeric', timeZone: zone }).format(d)
  return dateFormat(locale, {
    day: 'numeric',
    month: 'short',
    ...(year(at, timeZone) === year(new Date(now), nowZone) ? {} : { year: 'numeric' }),
    timeZone,
  }).format(at)
}

// gramDigits is how many decimals grams are shown with: one below 10 g,
// which a whole number would bend (1.5 g of fat is not 2), none from
// 10 g.
const gramDigits = (n: number) => (Math.abs(n) < 10 ? 1 : 0)

// formatGrams is grams as the app shows them, in a language (gramDigits).
// kcal are whole (number).
export function formatGrams(n: number, locale: string): string {
  return numberFormat(locale, { maximumFractionDigits: gramDigits(n) }).format(n)
}

// rounded is n as a number shows it, rounded to at most digits decimals,
// by the formatter that shows it: numbers worked out of what is shown
// then add up as shown (58.7 g eaten, shown 59, of 60 leave 1, not
// 1.3). Never -0, which would show as "-0".
export function rounded(n: number, digits = 0): number {
  const r = Number(
    numberFormat('en', { maximumFractionDigits: digits, useGrouping: false }).format(n),
  )
  return r === 0 ? 0 : r
}

// roundedGrams is grams as formatGrams shows them.
export function roundedGrams(n: number): number {
  return rounded(n, gramDigits(n))
}

// useFormat formats numbers, dates and times in the app's language.
export function useFormat() {
  const locale = useLocale()
  return {
    // number rounds to at most digits decimals.
    number: (n: number, digits = 0) =>
      numberFormat(locale, { maximumFractionDigits: digits }).format(n),
    // signed is a difference, its sign always said but of 0: −490, +0.6.
    signed: (n: number, digits = 0) =>
      numberFormat(locale, { maximumFractionDigits: digits, signDisplay: 'exceptZero' }).format(n),
    // weekday is a day's weekday, short: "Tue" in English, "uto" in
    // Croatian; dayOfMonth its number in the month.
    weekday: (date: string) =>
      dateFormat(locale, { weekday: 'short', timeZone: 'UTC' }).format(calendarDate(date)),
    dayOfMonth: (date: string) =>
      dateFormat(locale, { day: 'numeric', timeZone: 'UTC' }).format(calendarDate(date)),
    // grams are grams of a macro (formatGrams).
    grams: (n: number) => formatGrams(n, locale),
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
    // "srijeda, 22. listopada" in Croatian; of another year than today's
    // (YYYY-MM-DD, the user's; in UTC when not known), with it.
    date: (date: string, today: string = new Date().toISOString().slice(0, 10)) =>
      dateFormat(locale, {
        weekday: 'long',
        day: 'numeric',
        month: 'long',
        ...(date.slice(0, 4) === today.slice(0, 4) ? {} : { year: 'numeric' }),
        timeZone: 'UTC',
      }).format(calendarDate(date)),
    // short is a day's date, short: "Oct 21" in English, "21. lis" in
    // Croatian.
    short: (date: string) =>
      dateFormat(locale, { day: 'numeric', month: 'short', timeZone: 'UTC' }).format(
        calendarDate(date),
      ),
    // time is an instant's time of day on the user's clock.
    time: (instant: string, timeZone: string) =>
      dateFormat(locale, { hour: 'numeric', minute: '2-digit', timeZone }).format(
        new Date(instant),
      ),
    // day is an instant's day on the user's clock, short: "Oct 21" in
    // English, "21. lis" in Croatian; of another year than now's, with
    // it.
    day: (instant: string, timeZone: string, now: number = Date.now()) =>
      shortDay(locale, new Date(instant), timeZone, now),
    // calendarDay is a day's date (YYYY-MM-DD) as day says an instant's:
    // of another year than now's on the user's clock, with it.
    calendarDay: (date: string, timeZone: string, now: number = Date.now()) =>
      shortDay(locale, calendarDate(date), 'UTC', now, timeZone),
  }
}
