import { ChevronRight } from 'lucide-react'
import { useState } from 'react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import {
  budgetOf,
  floorOf,
  plannedTarget,
  weekAverage,
  weekDates,
  weekOf,
  type WeekAverage,
} from '@/components/day/week'
import { Sheet } from '@/components/sheet'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

type HistoryDay = components['schemas']['HistoryDayBody']

// WeekLine is the week of the day shown in a line: what its days logged
// ate a day on average, against their target, and how far under or over
// it a day, as the progress's weeks count them (a day without a meal, a
// day left out, and today while it is open are not counted, and the line
// says how many are). A tap shows the week's days in a sheet, each with
// what it ate and its target, and of this week, what the days left,
// today's among them, can eat on average and keep the week on its
// target; past the plan's kcal floor, the day's target instead. Nothing
// of a week to come, nor of one with neither a day logged nor a target.
export function WeekLine({ date, today }: { date: string; today: string }) {
  const t = useMessages()
  const f = useFormat()
  const [open, setOpen] = useState(false)
  const query = weekOf(date, today)
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query } },
    { enabled: query.from <= query.to },
  )
  const plan = $api.useQuery('get', '/api/v1/plan')
  const days = history.data?.days
  if (query.from > query.to || days === undefined) {
    return null
  }
  const average = weekAverage(days)
  if (average.days === 0 && !days.some((d) => d.target !== undefined)) {
    return null
  }
  const w = t.day.week
  const dates = weekDates(date)
  const name = dates.includes(today) ? w.thisWeek : w.thatWeek
  const byDate = new Map(days.map((d) => [d.date, d]))
  const budget = budgetOf(dates, byDate, today, (d) =>
    plan.data === undefined ? undefined : plannedTarget(plan.data, d),
  )
  const first = dates[0] ?? date
  const last = dates[6] ?? date

  // noteOf is what a day of the week is, when it is not counted.
  function noteOf(d: string, day: HistoryDay | undefined): string | undefined {
    if (d > today) {
      return w.toCome
    }
    if (d === today) {
      return w.today
    }
    if (day?.status === 'excluded') {
      return t.day.excluded('')
    }
    return day?.intake === undefined ? w.nothing : undefined
  }

  let advice: string | undefined
  if (budget !== undefined) {
    const lastDay = budget.days === 1
    advice =
      budget.excess > 0 && budget.kcal < floorOf(plan.data)
        ? (lastDay ? w.keepToday : w.keep)(f.number(budget.excess))
        : (lastDay ? w.budgetToday : w.budget)(f.number(budget.kcal))
  }

  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        onClick={() => {
          setOpen(true)
        }}
        className="flex min-h-11 w-full items-center gap-2 rounded-md bg-surface-2 px-3 py-2 text-left text-small outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
      >
        <span className="flex-1">
          <Said name={name} average={average} />
        </span>
        <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
      </button>
      <Sheet open={open} onOpenChange={setOpen} title={name}>
        <div className="space-y-4">
          <div className="space-y-1">
            <p className="text-small text-muted-foreground">
              {w.span(f.short(first), f.short(last))}
            </p>
            <p className="text-small">
              <Said name={name} average={average} />
            </p>
          </div>
          <table className="w-full text-small">
            <caption className="sr-only">{w.days}</caption>
            <thead className="text-caption text-muted-foreground">
              <tr>
                <th scope="col" className="pb-2 text-left font-medium">
                  {w.dayColumn}
                </th>
                <th scope="col" className="pb-2 pl-2 text-right font-medium">
                  {w.eatenColumn}
                </th>
                <th scope="col" className="pb-2 pl-2 text-right font-medium">
                  {w.targetColumn}
                </th>
              </tr>
            </thead>
            <tbody>
              {dates.map((d) => {
                const day = byDate.get(d)
                const target =
                  d > today && plan.data !== undefined
                    ? plannedTarget(plan.data, d)
                    : day?.target?.kcal
                const note = noteOf(d, day)
                return (
                  <tr key={d} className="border-t">
                    <th scope="row" className="py-2 text-left font-normal">
                      {f.weekday(d)} {f.dayOfMonth(d)}
                      {note !== undefined && (
                        <span className="block text-caption text-muted-foreground">{note}</span>
                      )}
                    </th>
                    <td className="py-2 pl-2 text-right font-semibold tabular-nums">
                      {day?.intake === undefined ? '—' : f.number(day.intake.kcal)}
                    </td>
                    <td className="py-2 pl-2 text-right text-muted-foreground tabular-nums">
                      {target === undefined ? '—' : f.number(target)}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
          {advice !== undefined && <p className="text-small">{advice}</p>}
        </div>
      </Sheet>
    </>
  )
}

// Said is the week's line: its average, bolder, against its target and
// how far from it a day, and how many days it counts.
function Said({ name, average }: { name: string; average: WeekAverage }) {
  const t = useMessages()
  const f = useFormat()
  const w = t.day.week
  if (average.kcal === undefined) {
    return <>{w.none(name)}</>
  }
  let from: string | undefined
  if (average.target !== undefined) {
    const under = average.target - average.kcal
    if (under > 0) {
      from = w.under(f.number(under))
    } else {
      from = under < 0 ? w.over(f.number(-under)) : w.on
    }
  }

  // A line breaks between its parts, never within one ("174 under a
  // day"); the spaces between them are the line's own, which a screen
  // reader says too.
  return (
    <>
      {w.average(name)} <b className="font-semibold">{f.number(average.kcal)}</b>
      {average.target !== undefined && (
        <>
          {' '}
          <span className="whitespace-nowrap">{w.of(f.number(average.target))}</span>
          {' · '}
          <span className="whitespace-nowrap">{from}</span>
        </>
      )}
      {' · '}
      <span className="whitespace-nowrap text-muted-foreground">{w.logged(average.days)}</span>
    </>
  )
}
