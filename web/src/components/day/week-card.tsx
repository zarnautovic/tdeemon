import { $api } from '@/api/client'
import { Legend } from '@/components/charts/legend'
import { weekAverage, weekDates, weekOf } from '@/components/day/week'
import { WeekLine } from '@/components/day/week-line'
import { WeekStrip } from '@/components/day/week-strip'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// WeekCard is a desktop's week, beside the day: its days as bars of what
// each ate, with its target a dashed tick, and the average of the days
// logged a line across them (WeekStrip); then the week in a line, which
// opens its days and what the days left can eat (WeekLine).
export function WeekCard({ date, today }: { date: string; today: string }) {
  const t = useMessages()
  const f = useFormat()
  const query = weekOf(date, today)
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query } },
    { enabled: query.from <= query.to },
  )
  const average = history.data === undefined ? undefined : weekAverage(history.data.days).kcal
  const dates = weekDates(date)
  const w = t.day.week

  return (
    <section aria-labelledby="day-week" className="space-y-3 rounded-lg border bg-card p-4">
      <div className="flex items-baseline justify-between gap-2">
        <h2 id="day-week" className="text-h3">
          {dates.includes(today) ? w.thisWeek : w.thatWeek}
        </h2>
        <span className="text-caption text-muted-foreground">
          {w.span(f.short(dates[0] ?? date), f.short(dates[6] ?? date))}
        </span>
      </div>
      <WeekStrip date={date} today={today} average={average} />
      <Legend
        marks={[
          { name: w.legendTarget, mark: 'dashed', color: 'border-foreground/60' },
          ...(average === undefined
            ? []
            : [{ name: w.legendAverage, mark: 'line' as const, color: 'bg-muted-foreground' }]),
        ]}
      />
      <WeekLine date={date} today={today} />
    </section>
  )
}
