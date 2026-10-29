import { keepPreviousData } from '@tanstack/react-query'
import { useId, useState } from 'react'

import { $api } from '@/api/client'
import { LoadError } from '@/components/load-error'
import { Skeleton } from '@/components/progress/skeleton'
import { card, entry } from '@/components/progress/styles'
import { Button } from '@/components/ui/button'
import { daysBetween, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// The weeks shown before the rest is asked for.
const first = 8

// WeeklyTable is the span's calendar weeks, Monday to Sunday, the latest
// first, as calibration counts them (GET /api/v1/history by week): the
// kcal and the protein eaten a day on average over the days logged, the
// trend weight's change from the week before, and how many of its days
// were logged, of the 7, or of this week's that are over. It says in text
// what the charts show. weeks is the history to ask for, a week more than
// it shows: the first's change is from it.
export function WeeklyTable({
  weeks,
  today,
}: {
  weeks: { from: string; to: string }
  today: string
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const [all, setAll] = useState(false)
  const w = t.charts.weeks
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query: { from: weeks.from, to: weeks.to, granularity: 'week' } } },
    { placeholderData: keepPreviousData },
  )

  let body
  if (history.data === undefined && history.isError) {
    body = <LoadError error={history.error} onRetry={() => void history.refetch()} />
  } else if (!history.data) {
    body = (
      <div aria-busy className="space-y-2">
        <output className="sr-only">{t.loading}</output>
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-7" />
        ))}
      </div>
    )
  } else {
    const rows = history.data.weeks
      .flatMap((week, i) => {
        const before = history.data.weeks[i - 1]
        if (before === undefined) {
          return []
        }
        const change =
          week.trend_kg === undefined || before.trend_kg === undefined
            ? undefined
            : week.trend_kg - before.trend_kg
        return [{ ...week, change }]
      })
      .toReversed()
    const shown = all ? rows : rows.slice(0, first)
    const none = w.none
    body = (
      <>
        <table
          aria-labelledby={id}
          aria-busy={history.isPlaceholderData}
          className={cn('w-full text-small', history.isPlaceholderData && 'opacity-60')}
        >
          <thead className="text-caption text-muted-foreground">
            <tr>
              <th scope="col" className="pb-2 text-left font-medium">
                {w.week}
              </th>
              {[
                [w.kcal, w.kcalSaid],
                [w.protein, w.proteinSaid],
                [w.change, w.changeSaid],
                [w.days, w.daysSaid],
              ].map(([short, long]) => (
                <th key={short} scope="col" className="pb-2 pl-2 text-right font-medium">
                  <span aria-hidden>{short}</span>
                  <span className="sr-only">{long}</span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {shown.map((week) => (
              <tr key={week.from} className="border-t">
                <th scope="row" className="py-2.5 text-left font-normal whitespace-nowrap">
                  {/* With its year when another than today's. */}
                  {f.days(week.from, week.to, today)}
                </th>
                <td className="py-2.5 pl-2 text-right font-semibold">
                  {week.kcal === undefined ? none : f.number(week.kcal)}
                </td>
                <td className="py-2.5 pl-2 text-right">
                  {week.protein === undefined ? none : f.number(week.protein)}
                </td>
                <td className="py-2.5 pl-2 text-right whitespace-nowrap">
                  {week.change === undefined ? none : f.signed(week.change, 1)}
                </td>
                <td className="py-2.5 pl-2 text-right text-muted-foreground">
                  {w.daysOf(week.logged_days, daysBetween(week.from, week.to) + 1)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {rows.length > first && (
          <Button
            variant="ghost"
            size="sm"
            className="mt-2 w-full"
            aria-expanded={all}
            onClick={() => {
              setAll((a) => !a)
            }}
          >
            {all ? w.fewer : w.all(rows.length)}
          </Button>
        )}
      </>
    )
  }

  return (
    <section aria-labelledby={id} className={cn(card, 'space-y-3', entry)}>
      <h2 id={id} className="text-h3">
        {w.title}
      </h2>
      {body}
    </section>
  )
}
