import { Link } from '@tanstack/react-router'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { weekOf } from '@/components/day/week'
import { addDays, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

type HistoryDay = components['schemas']['HistoryDayBody']

// The mini ring's circle, in its viewBox's units, and its length.
const radius = 11
const length = 2 * Math.PI * radius

// MiniRing is a day's intake against its target, as the day's ring has
// it on a smaller scale: filled to it, a destructive arc on past it; a
// day without a meal is a dashed circle, and one without a target a full
// muted one.
function MiniRing({ day }: { day: HistoryDay | undefined }) {
  const intake = day?.intake?.kcal
  const target = day?.target?.kcal
  const share = intake !== undefined && target ? Math.min(intake / target, 1) : 0
  const beyond = intake !== undefined && target ? Math.min(Math.max(intake / target - 1, 0), 1) : 0
  const arc = (part: number, className: string) => (
    <circle
      cx="14"
      cy="14"
      r={radius}
      fill="none"
      strokeWidth="4"
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
      {share > 0 && arc(share, 'stroke-primary')}
      {beyond > 0 && arc(beyond, 'stroke-destructive')}
    </svg>
  )
}

// Bar is a day's intake as a bar, against the week's most (scale), with
// its target a tick across: a desktop's week, the days side by side.
function Bar({ day, scale }: { day: HistoryDay | undefined; scale: number }) {
  const intake = day?.intake?.kcal
  const target = day?.target?.kcal
  const over = intake !== undefined && target !== undefined && intake > target

  return (
    <span aria-hidden className="relative hidden h-16 w-6 rounded-sm bg-surface-2 lg:block">
      {intake !== undefined && (
        <span
          className={`absolute inset-x-0 bottom-0 rounded-sm ${over ? 'bg-destructive' : 'bg-primary'}`}
          style={{ height: `${String((intake / scale) * 100)}%` }}
        />
      )}
      {target !== undefined && (
        <span
          className="absolute -inset-x-1 border-t-2 border-dashed border-foreground/60"
          style={{ bottom: `${String((target / scale) * 100)}%` }}
        />
      )}
    </span>
  )
}

// WeekStrip is the seven days up to today, or up to the day shown when it
// is before them: each a link to its day, with what was eaten of its
// target, as a mini ring on a phone and as bars on a desktop; the day
// shown is the current one, inverted (in the dark, the brand's tint).
// Each says it in words.
export function WeekStrip({ date, today }: { date: string; today: string }) {
  const t = useMessages()
  const f = useFormat()
  const { from, to } = weekOf(date, today)
  const history = $api.useQuery('get', '/api/v1/history', { params: { query: { from, to } } })
  const byDate = new Map(history.data?.days.map((d) => [d.date, d]))
  const dates = Array.from({ length: 7 }, (_, i) => addDays(from, i))
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

  return (
    <nav aria-label={t.day.week.label}>
      <ol className="grid grid-cols-7 gap-1">
        {dates.map((d) => {
          const day = byDate.get(d)
          return (
            <li key={d}>
              <Link
                to="/"
                // Today's link is / itself.
                search={d === today ? {} : { date: d }}
                activeOptions={{ exact: true }}
                // Named as it reads, its weekday and day, so a voice
                // says what it sees.
                aria-label={t.day.week.day(`${f.weekday(d)} ${f.dayOfMonth(d)}`, said(day))}
                className="group flex flex-col items-center gap-1 rounded-md py-1.5 text-muted-foreground outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-foreground aria-[current=page]:text-background dark:aria-[current=page]:bg-primary-soft dark:aria-[current=page]:text-primary"
              >
                {/* A space after the weekday, which a phone does not show
                    in a column, and the text reads "Tue 22", as the name. */}
                <span aria-hidden className="text-caption">
                  {f.weekday(d)}{' '}
                </span>
                <MiniRing day={day} />
                <Bar day={day} scale={scale} />
                <span
                  aria-hidden
                  className="text-caption font-semibold text-foreground group-aria-[current=page]:text-background dark:group-aria-[current=page]:text-primary"
                >
                  {f.dayOfMonth(d)}
                </span>
              </Link>
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
