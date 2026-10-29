import { useFormat } from '@/i18n/format'
import { useLocale } from '@/i18n/locale'
import { useMessages } from '@/i18n/messages'

import type { Words } from './options'

// months keeps a language's formatter of a month and its year.
const months = new Map<string, Intl.DateTimeFormat>()

// monthOf is a date's month and year, short, in a language: "Oct 2026"
// in English, "lis 2026." in Croatian.
export function monthOf(date: string, locale: string): string {
  let format = months.get(locale)
  if (format === undefined) {
    format = new Intl.DateTimeFormat(locale, { month: 'short', year: 'numeric', timeZone: 'UTC' })
    months.set(locale, format)
  }
  return format.format(new Date(`${date}T00:00:00Z`))
}

// useChartWords are the charts' words in the app's language. Over a year
// (byMonth) the axis names months and their year, not days.
export function useChartWords(byMonth: boolean): Words {
  const t = useMessages()
  const f = useFormat()
  const locale = useLocale()

  return {
    axisDate: byMonth ? (date) => monthOf(date, locale) : f.short,
    months: byMonth,
    day: (date) => t.charts.day(f.weekday(date), f.short(date)),
    number: f.number,
    percent: f.percent,
    hm: (hours) => t.unit.hm(...f.hm(hours)),
    names: t.charts.names,
    tips: t.charts.tips,
  }
}
