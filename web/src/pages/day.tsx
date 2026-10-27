import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { ChevronLeft, ChevronRight } from 'lucide-react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import { CheckIn } from '@/components/day/check-in'
import { DayMetrics } from '@/components/day/day-metrics'
import { DayTotals } from '@/components/day/day-totals'
import { MealCard } from '@/components/day/meal-card'
import { MealLog } from '@/components/day/meal-log'
import { LoadError } from '@/components/load-error'
import { buttonVariants } from '@/components/ui/button'
import { addDays, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { useShortcuts } from '@/lib/shortcuts'
import type { DaySearch } from '@/router'

// DayKeys moves a day back or ahead by the keyboard's arrows, as the
// page's arrows do: to the day before, and to the day after up to today.
function DayKeys({ before, after }: { before: DaySearch; after: DaySearch | undefined }) {
  const navigate = useNavigate()
  useShortcuts({
    ArrowLeft: () => {
      void navigate({ to: '/', search: before })
    },
    ...(after && {
      ArrowRight: () => {
        void navigate({ to: '/', search: after })
      },
    }),
  })
  return null
}

// DayPage is a day of the user's: today at /, another at /?date=YYYY-MM-DD.
// It shows what was eaten against the target, the check-in, the meals and
// the measurements, takes the check-in and the measurements, and moves a
// day back or ahead, up to today. /?log=true opens the form to log a meal
// on today (the app's Log a meal).
export function DayPage() {
  const t = useMessages()
  const f = useFormat()
  const { date, log } = useSearch({ from: '/app/' })
  const navigate = useNavigate()
  const day = $api.useQuery(
    'get',
    '/api/v1/days/{date}',
    { params: { path: { date: date ?? 'today' } } },
    // Moving to another day keeps this one, and its arrows under the
    // thumb, until the other comes.
    { placeholderData: keepPreviousData },
  )
  const me = $api.useQuery('get', '/api/v1/me')
  const session = useQuery(sessionQuery)
  const today = me.data?.today

  if (day.isPending) {
    return <output className="block text-muted-foreground">{t.loading}</output>
  }
  if (day.isError) {
    return <LoadError error={day.error} onRetry={() => void day.refetch()} />
  }

  // While another day comes, the one before stays on the page
  // (placeholder data), and the title and arrows already follow the day
  // asked for.
  const waiting = day.isPlaceholderData
  // Today is the day / answered, once it has: /me's may be older, the
  // day's start hour having passed since.
  const todayDate = date === undefined && !waiting ? day.data.date : today
  const shown = date ?? todayDate ?? day.data.date
  const isToday = date === undefined || shown === todayDate
  // A day to come, which an address may name though the arrows stop at
  // today, takes its type and exclusion only, as the server has it. A day
  // an address names may be one until the app knows today; if it cannot
  // learn it, the server says what it refuses.
  const future = todayDate === undefined ? date !== undefined && me.isPending : shown > todayDate
  const before = addDays(shown, -1)
  const after = addDays(shown, 1)
  // A day's link: today's is / itself.
  const searchOf = (d: string) => (d === todayDate ? {} : { date: d })
  const hasNext = todayDate !== undefined && shown < todayDate

  return (
    <div className="max-w-3xl space-y-4">
      <DayKeys before={searchOf(before)} after={hasNext ? searchOf(after) : undefined} />
      <header className="flex items-center gap-2">
        <Link
          to="/"
          search={searchOf(before)}
          // Another day is never the current page: its date must match
          // too, not be left out.
          activeOptions={{ exact: true }}
          aria-label={t.day.previous}
          className={buttonVariants({ variant: 'ghost', size: 'icon' })}
        >
          <ChevronLeft aria-hidden />
        </Link>
        <div className="flex-1 text-center">
          <h1 className="text-xl font-semibold">
            {isToday ? t.day.today : f.date(shown, todayDate)}
          </h1>
          <p className="text-sm text-muted-foreground">
            {isToday && `${f.date(shown, todayDate)} · `}
            {day.data.type === 'training' ? t.day.training : t.day.rest}
          </p>
        </div>
        {hasNext ? (
          <Link
            to="/"
            search={searchOf(after)}
            activeOptions={{ exact: true }}
            aria-label={t.day.next}
            className={buttonVariants({ variant: 'ghost', size: 'icon' })}
          >
            <ChevronRight aria-hidden />
          </Link>
        ) : (
          // Keeps the title centred.
          <span className="size-9" />
        )}
      </header>
      <div
        aria-busy={waiting}
        // What is changed while it waits would be the day before's.
        inert={waiting}
        className={`space-y-4 ${waiting ? 'opacity-60' : ''}`}
      >
        <DayTotals day={day.data} />
        {/* Its fields start again with each day. */}
        <CheckIn key={`check-in ${day.data.date}`} day={day.data} future={future} />
        <section aria-labelledby="day-meals" className="space-y-2">
          <h2 id="day-meals" className="font-medium">
            {t.day.meals}
          </h2>
          {day.data.meals.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t.day.noMeals}</p>
          ) : (
            day.data.meals.map((meal) => (
              <MealCard
                key={meal.id}
                meal={meal}
                date={day.data.date}
                today={todayDate ?? day.data.date}
                timeZone={session.data?.user.timezone ?? 'UTC'}
                startHour={session.data?.user.day_start_hour ?? 0}
              />
            ))
          )}
          {!future && (
            <MealLog
              key={`log ${day.data.date}`}
              opening={log === true}
              onOpened={() => {
                void navigate({ to: '/', search: {}, replace: true })
              }}
              date={day.data.date}
              today={isToday}
              timeZone={session.data?.user.timezone ?? 'UTC'}
              startHour={session.data?.user.day_start_hour ?? 0}
            />
          )}
        </section>
        <DayMetrics
          key={`metrics ${day.data.date}`}
          date={day.data.date}
          metrics={day.data.metrics}
          canLog={!future}
        />
      </div>
      {!isToday && (
        <Link
          to="/"
          activeOptions={{ exact: true }}
          className={buttonVariants({ variant: 'outline', className: 'w-full' })}
        >
          {t.day.toToday}
        </Link>
      )}
    </div>
  )
}
