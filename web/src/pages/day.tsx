import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link, useSearch } from '@tanstack/react-router'
import { ChevronLeft, ChevronRight } from 'lucide-react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import { DayMetrics } from '@/components/day/day-metrics'
import { DayTotals } from '@/components/day/day-totals'
import { MealCard } from '@/components/day/meal-card'
import { LoadError } from '@/components/load-error'
import { buttonVariants } from '@/components/ui/button'
import { addDays, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// DayPage is a day of the user's: today at /, another at /?date=YYYY-MM-DD.
// It shows what was eaten against the target, the meals and the
// measurements, and moves a day back or ahead, up to today.
export function DayPage() {
  const t = useMessages()
  const f = useFormat()
  const { date } = useSearch({ from: '/app/' })
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

  const shown = day.data.date
  // / is today, whatever /me said earlier: the day's start hour may have
  // passed since.
  const isToday = date === undefined || shown === today
  const before = addDays(shown, -1)
  const after = addDays(shown, 1)
  // A day's link: today's is / itself.
  const searchOf = (d: string) => (d === today ? {} : { date: d })

  return (
    <div className="space-y-4">
      <header className="flex items-center gap-2">
        <Link
          to="/"
          search={searchOf(before)}
          // Another day is never the current page: its date must match
          // too, not be left out.
          activeOptions={{ exact: true }}
          aria-label={t.day.previous}
          className={buttonVariants({ variant: 'ghost', size: 'icon-lg' })}
        >
          <ChevronLeft aria-hidden />
        </Link>
        <div className="flex-1 text-center">
          <h1 className="text-xl font-semibold">{isToday ? t.day.today : f.date(shown)}</h1>
          <p className="text-sm text-muted-foreground">
            {isToday && `${f.date(shown)} · `}
            {day.data.type === 'training' ? t.day.training : t.day.rest}
          </p>
        </div>
        {today !== undefined && shown < today ? (
          <Link
            to="/"
            search={searchOf(after)}
            activeOptions={{ exact: true }}
            aria-label={t.day.next}
            className={buttonVariants({ variant: 'ghost', size: 'icon-lg' })}
          >
            <ChevronRight aria-hidden />
          </Link>
        ) : (
          // Keeps the title centred.
          <span className="size-9" />
        )}
      </header>
      {day.data.status === 'excluded' && (
        <p className="rounded-lg bg-muted px-3 py-2 text-sm">
          {t.day.excluded(day.data.excluded_reason ?? '')}
        </p>
      )}
      <DayTotals day={day.data} />
      <section aria-labelledby="day-meals" className="space-y-2">
        <h2 id="day-meals" className="font-medium">
          {t.day.meals}
        </h2>
        {day.data.meals.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t.day.noMeals}</p>
        ) : (
          day.data.meals.map((meal) => (
            <MealCard key={meal.id} meal={meal} timeZone={session.data?.user.timezone ?? 'UTC'} />
          ))
        )}
      </section>
      <DayMetrics metrics={day.data.metrics} />
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
