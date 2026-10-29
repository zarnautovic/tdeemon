import { Link } from '@tanstack/react-router'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { kcalStanding } from '@/components/day/left'
import { weekDates, weekOf } from '@/components/day/week'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

type HistoryDay = components['schemas']['HistoryDayBody']

// The mini ring's circle, in its viewBox's units, and its length.
const radius = 11
const length = 2 * Math.PI * radius

// overOf is how far past its target a day's intake is, as a share of the
// target, when it is past it by more than the week takes in its stride
// (kcalStanding); 0 when it is not.
function overOf(intake: number | undefined, target: number | undefined): number {
  return intake !== undefined && target && kcalStanding(intake, target) === 'over'
    ? intake / target - 1
    : 0
}

// MiniRing is a day's intake against its target, as the day's ring has
// it on a smaller scale: filled to it, and a thinner arc in the over
// colour on past it, by more than the week takes in its stride; a day
// without a meal is a dashed circle, and one without a target a full
// muted one.
function MiniRing({ day }: { day: HistoryDay | undefined }) {
  const intake = day?.intake?.kcal
  const target = day?.target?.kcal
  const share = intake !== undefined && target ? Math.min(intake / target, 1) : 0
  const beyond = Math.min(overOf(intake, target), 1)
  const arc = (part: number, width: number, className: string) => (
    <circle
      cx="14"
      cy="14"
      r={radius}
      fill="none"
      strokeWidth={width}
      strokeDasharray={`${String(part * length)} ${String(length)}`}
      className={className}
    />
  )

  return (
    <svg aria-hidden viewBox="0 0 28 28" className="size-7 -rotate-90 lg:hidden">
      <circle
        cx="14"
        cy="14"
        r={radius}
        fill="none"
        strokeWidth="4"
        strokeDasharray={intake === undefined ? '3 3' : undefined}
        className="stroke-border"
      />
      {share > 0 && arc(share, 4, 'stroke-primary')}
      {beyond > 0 && arc(beyond, 2, 'stroke-over')}
    </svg>
  )
}

// Bar is a day's intake as a bar, against the week's most (scale), with
// its target a tick across: a desktop's week, the days side by side. Past
// its target by more than the week takes in its stride, the bar is the
// brand's up to the tick and, above it, narrower in the over colour. The
// week's average, when it has one, is a line across the day's column,
// which meets the next day's over the gap between them.
function Bar({
  day,
  scale,
  average,
}: {
  day: HistoryDay | undefined
  scale: number
  average: number | undefined
}) {
  const intake = day?.intake?.kcal
  const target = day?.target?.kcal
  const over = overOf(intake, target) > 0
  const below = over && target !== undefined ? target : intake

  return (
    <span aria-hidden className="relative hidden h-20 w-full lg:block">
      <span className="absolute inset-y-0 left-1/2 w-6 -translate-x-1/2 rounded-sm bg-surface-2">
        {below !== undefined && (
          <span
            className="absolute inset-x-0 bottom-0 rounded-sm bg-primary"
            style={{ height: `${String((below / scale) * 100)}%` }}
          />
        )}
        {over && intake !== undefined && below !== undefined && (
          <span
            className="absolute inset-x-1.5 rounded-t-sm bg-over"
            style={{
              bottom: `${String((below / scale) * 100)}%`,
              height: `${String(((intake - below) / scale) * 100)}%`,
            }}
          />
        )}
        {target !== undefined && (
          <span
            className="absolute -inset-x-1 border-t-2 border-dashed border-foreground/60"
            style={{ bottom: `${String((target / scale) * 100)}%` }}
          />
        )}
      </span>
      {average !== undefined && (
        <span
          data-average
          className="absolute -inset-x-0.5 border-t-2 border-muted-foreground group-aria-[current]:border-background dark:group-aria-[current]:border-primary"
          style={{ bottom: `${String((average / scale) * 100)}%` }}
        />
      )}
    </span>
  )
}

// WeekStrip is the calendar week of the day shown, Monday to Sunday, as
// the progress's weeks are: each day a link to its day, with what was
// eaten of its target, as a mini ring on a phone and as bars on a
// desktop, where average, the week's, is a line across them; the day
// shown is the current one, inverted (in the dark, the brand's tint). A
// day to come is no link, as the arrows stop at today, and has nothing
// to show. Each says it in words.
export function WeekStrip({
  date,
  today,
  average,
}: {
  date: string
  today: string
  average?: number | undefined
}) {
  const t = useMessages()
  const f = useFormat()
  const query = weekOf(date, today)
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query } },
    // A week to come has no history yet.
    { enabled: query.from <= query.to },
  )
  const byDate = new Map(history.data?.days.map((d) => [d.date, d]))
  const dates = weekDates(date)
  const scale = Math.max(
    1,
    ...dates.map((d) =>
      Math.max(byDate.get(d)?.intake?.kcal ?? 0, byDate.get(d)?.target?.kcal ?? 0),
    ),
  )

  function said(day: HistoryDay | undefined): string {
    const intake = day?.intake?.kcal
    if (intake === undefined) {
      return t.day.week.nothing
    }
    const target = day?.target?.kcal
    return target === undefined
      ? t.day.week.eaten(f.number(intake))
      : t.day.week.eatenOf(f.number(intake), f.number(target))
  }

  const cell = 'group flex flex-col items-center gap-1 rounded-md py-1.5 text-muted-foreground'

  return (
    <nav aria-label={t.day.week.label}>
      <ol className="grid grid-cols-7 gap-1">
        {dates.map((d) => {
          const day = byDate.get(d)
          // Named as it reads, its weekday and day, so a voice says what
          // it sees.
          const name = `${f.weekday(d)} ${f.dayOfMonth(d)}`
          const face = (
            <>
              {/* A space after the weekday, which a phone does not show
                  in a column, and the text reads "Tue 22", as the name. */}
              <span aria-hidden className="text-caption">
                {f.weekday(d)}{' '}
              </span>
              <MiniRing day={day} />
              <Bar day={day} scale={scale} average={average} />
              <span
                aria-hidden
                className="text-caption font-semibold text-foreground group-aria-[current]:text-background dark:group-aria-[current]:text-primary"
              >
                {f.dayOfMonth(d)}
              </span>
            </>
          )
          return (
            <li key={d}>
              {d > today ? (
                // A day to come, the day shown only by an address.
                <span
                  aria-current={d === date ? 'date' : undefined}
                  className={cn(
                    cell,
                    'aria-[current]:bg-foreground aria-[current]:text-background dark:aria-[current]:bg-primary-soft dark:aria-[current]:text-primary',
                  )}
                >
                  <span className="sr-only">{t.day.week.day(name, t.day.week.toCome)}</span>
                  {face}
                </span>
              ) : (
                <Link
                  to="/"
                  // Today's link is / itself.
                  search={d === today ? {} : { date: d }}
                  activeOptions={{ exact: true }}
                  aria-label={t.day.week.day(name, said(day))}
                  className={cn(
                    cell,
                    'outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-[current]:bg-foreground aria-[current]:text-background dark:aria-[current]:bg-primary-soft dark:aria-[current]:text-primary',
                  )}
                >
                  {face}
                </Link>
              )}
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
