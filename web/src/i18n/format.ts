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
    // signedPercent is a difference in percent, its sign always said but
    // of 0: −7 %, +3 %.
    signedPercent: (n: number, digits = 0) =>
      numberFormat(locale, {
        style: 'percent',
        maximumFractionDigits: digits,
        signDisplay: 'exceptZero',
      }).format(n / 100),
    // list is words in a list, with the language's "and": "sleep and
    // HRV", "san i HRV".
    list: (words: string[]) => new Intl.ListFormat(locale, { type: 'conjunction' }).format(words),
    // hm is hours as whole hours and minutes, each as the language writes
    // numbers, the minutes in two digits: 6.87 hours are 6 and 52.
    hm: (hours: number): [string, string] => {
      const minutes = Math.round(hours * 60)
      return [
        numberFormat(locale, {}).format(Math.floor(minutes / 60)),
        numberFormat(locale, { minimumIntegerDigits: 2, useGrouping: false }).format(minutes % 60),
      ]
    },
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
    // days are the days from one date to another, short: "Oct 19 – 25" in
    // English, "19. – 25. lis" in Croatian; of another year than today's
    // (YYYY-MM-DD, the user's), with it.
    days: (from: string, to: string, today: string) =>
      dateFormat(locale, {
        day: 'numeric',
        month: 'short',
        ...(from.slice(0, 4) === today.slice(0, 4) && to.slice(0, 4) === today.slice(0, 4)
          ? {}
          : { year: 'numeric' }),
        timeZone: 'UTC',
      }).formatRange(calendarDate(from), calendarDate(to)),
    // time is an instant's time of day on the user's clock.
    time: (instant: string, timeZone: string) =>
      dateFormat(locale, { hour: 'numeric', minute: '2-digit', timeZone }).format(
        new Date(instant),
      ),
    // clock is a time of day (HH:MM) as time says one, to be read and
    // typed again (parseClock): 7:30 PM in English, 19:30 in Croatian,
    // its spaces plain where Intl puts a narrow one before PM.
    clock: (time: string) => {
      const [hour = 0, minute = 0] = time.split(':').map(Number)
      return dateFormat(locale, { hour: 'numeric', minute: '2-digit', timeZone: 'UTC' })
        .format(Date.UTC(1970, 0, 1, hour, minute))
        .replaceAll(/\s/g, ' ')
    },
    // hour12 tells a time on a 12-hour clock, 8:45 AM in English, which a
    // column of times takes more room for than 8:45 in Croatian.
    hour12:
      dateFormat(locale, { hour: 'numeric', timeZone: 'UTC' }).resolvedOptions().hour12 === true,
    // day is an instant's day on the user's clock, short: "Oct 21" in
    // English, "21. lis" in Croatian; of another year than now's, with
    // it.
    day: (instant: string, timeZone: string, now: number = Date.now()) =>
      shortDay(locale, new Date(instant), timeZone, now),
    // calendarDay is a day's date (YYYY-MM-DD) as day says an instant's:
    // of another year than now's on the user's clock, with it.
    calendarDay: (date: string, timeZone: string, now: number = Date.now()) =>
      shortDay(locale, calendarDate(date), 'UTC', now, timeZone),
    // ago is how long before now an instant was, short, in seconds,
    // minutes, hours or days: "5 sec. ago" or "2 hr. ago" in English,
    // "prije 5 s" or "prije 2 h" in Croatian.
    ago: (instant: string, now: number = Date.now()) => {
      const seconds = Math.max(0, Math.round((now - new Date(instant).getTime()) / 1000))
      const format = relativeFormat(locale)
      if (seconds < 60) {
        return format.format(-seconds, 'second')
      }
      const minutes = Math.round(seconds / 60)
      if (minutes < 60) {
        return format.format(-minutes, 'minute')
      }
      return minutes < 24 * 60
        ? format.format(-Math.round(minutes / 60), 'hour')
        : format.format(-Math.round(minutes / (24 * 60)), 'day')
    },
    // daysAgo is how long ago a day was, days before today, in words: in
    // days under two weeks, in weeks under two months, in months under a
    // year, and then in years, each the nearest whole one: "9 days ago",
    // "6 weeks ago" of 45 days, "prije 3 mjeseca" of 90.
    daysAgo: (days: number) => {
      const format = relativeFormat(locale, 'long')
      if (days < 14) {
        return format.format(-days, 'day')
      }
      if (days < 60) {
        return format.format(-Math.round(days / 7), 'week')
      }
      return days < 365
        ? format.format(-Math.round(days / daysInMonth), 'month')
        : format.format(-Math.round(days / daysInYear), 'year')
    },
  }
}

// The days of a year, and of a month, on average.
const daysInYear = 365.25
const daysInMonth = daysInYear / 12

// relativeFormats keeps each language's formatter of how long ago, short
// (5 sec. ago, yesterday) or long (5 days ago), once made.
const relativeFormats = new Map<string, Intl.RelativeTimeFormat>()

function relativeFormat(
  locale: string,
  style: 'short' | 'long' = 'short',
): Intl.RelativeTimeFormat {
  const key = `${locale}|${style}`
  let f = relativeFormats.get(key)
  if (f === undefined) {
    // Long, always in numbers: a year before is "1 year ago", not "last
    // year", which is a year of the calendar.
    f = new Intl.RelativeTimeFormat(locale, {
      style,
      numeric: style === 'short' ? 'auto' : 'always',
    })
    relativeFormats.set(key, f)
  }
  return f
}
