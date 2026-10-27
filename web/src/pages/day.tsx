import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { ChevronLeft, ChevronRight, Plus } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import type { Food } from '@/api/types'
import { BodyToday } from '@/components/day/body-today'
import { CheckIn } from '@/components/day/check-in'
import { DaySummary } from '@/components/day/day-summary'
import { DayTypeChip } from '@/components/day/day-type-chip'
import { MealCard } from '@/components/day/meal-card'
import { slotAt } from '@/components/day/meal-slot'
import { QuickAdd } from '@/components/day/quick-add'
import { QuickChips } from '@/components/day/quick-chips'
import { WeekStrip } from '@/components/day/week-strip'
import { LoadError } from '@/components/load-error'
import { Button, buttonVariants } from '@/components/ui/button'
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
// It shows the week's days, what was eaten against the target, the
// meals, the check-in and the measurements, takes the day's type, the
// check-in and the measurements, logs a meal (QuickAdd), and moves a day
// back or ahead, up to today. /?log=true opens the meal to log on today
// (the app's Log a meal).
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
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const today = me.data?.today
  // When the page opened, for the meal of the hour to add.
  const [now] = useState(Date.now)
  // The meal being logged, open, and the food it starts with; and what
  // opened it, which the focus goes back to.
  const [adding, setAdding] = useState<{ food?: Food } | undefined>(undefined)
  const opener = useRef<HTMLElement | null>(null)
  // Asked to open by the address: opened, and the address left as today's.
  const [asked, setAsked] = useState(false)
  if ((log === true) !== asked) {
    setAsked(log === true)
    if (log === true) {
      setAdding({})
    }
  }
  useEffect(() => {
    if (log === true) {
      // Opened from another page: the focus goes back where it was.
      opener.current = null
      void navigate({ to: '/', search: {}, replace: true })
    }
  }, [log, navigate])

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
    <div className="max-w-3xl space-y-4 lg:max-w-none">
      <DayKeys before={searchOf(before)} after={hasNext ? searchOf(after) : undefined} />
      <header className="flex items-center gap-2">
        <div className="flex shrink-0 items-center">
          <Link
            to="/"
            search={searchOf(before)}
            // Another day is never the current page: its date must match
            // too, not be left out.
            activeOptions={{ exact: true }}
            aria-label={t.day.previous}
            className={buttonVariants({ variant: 'ghost', size: 'icon-sm' })}
          >
            <ChevronLeft aria-hidden />
          </Link>
          {hasNext ? (
            <Link
              to="/"
              search={searchOf(after)}
              activeOptions={{ exact: true }}
              aria-label={t.day.next}
              className={buttonVariants({ variant: 'ghost', size: 'icon-sm' })}
            >
              <ChevronRight aria-hidden />
            </Link>
          ) : (
            // Keeps the title where it is.
            <span className="size-11 lg:size-9" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          {isToday ? (
            <>
              <h1 className="text-title">{t.day.today}</h1>
              <p className="text-small text-muted-foreground">{f.date(shown, todayDate)}</p>
            </>
          ) : (
            <h1 className="text-h2">{f.date(shown, todayDate)}</h1>
          )}
        </div>
        {/* Of the day shown, not of the one before while it comes. */}
        <div inert={waiting}>
          <DayTypeChip key={`type ${day.data.date}`} day={day.data} />
        </div>
      </header>
      {todayDate !== undefined && <WeekStrip date={shown} today={todayDate} />}
      <div
        aria-busy={waiting}
        // What is changed while it waits would be the day before's.
        inert={waiting}
        className={`grid grid-cols-1 gap-4 lg:grid-cols-12 lg:items-start ${waiting ? 'opacity-60' : ''}`}
      >
        {/* A desktop's columns: the day and its meals, and beside them
            what the user says of it and measures. A phone has them one
            after the other. */}
        <div className="space-y-4 lg:col-span-8">
          <DaySummary day={day.data} />
          {isToday && (
            <QuickChips
              date={day.data.date}
              timeZone={timeZone}
              now={now}
              onFood={(food, from) => {
                opener.current = from
                setAdding({ food })
              }}
            />
          )}
          <section aria-labelledby="day-meals" className="space-y-2">
            <div className="flex items-baseline justify-between gap-2">
              <h2 id="day-meals" className="text-h2">
                {t.day.meals}
              </h2>
              {day.data.meals.length > 0 && (
                <span className="text-small text-muted-foreground">
                  {t.day.mealsTotal(day.data.meals.length, f.number(day.data.total.kcal))}
                </span>
              )}
            </div>
            {day.data.meals.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t.day.noMeals}</p>
            ) : (
              <ul className="space-y-2">
                {day.data.meals.map((meal) => (
                  <li key={meal.id}>
                    <MealCard
                      meal={meal}
                      date={day.data.date}
                      today={todayDate ?? day.data.date}
                      timeZone={timeZone}
                      startHour={session.data?.user.day_start_hour ?? 0}
                    />
                  </li>
                ))}
              </ul>
            )}
            {!future && (
              <Button
                variant="outline"
                className="h-13 w-full border-2 border-dashed bg-transparent font-semibold text-primary"
                onClick={(e) => {
                  opener.current = e.currentTarget
                  setAdding({})
                }}
              >
                <Plus aria-hidden />
                {/* Today's names the meal of the hour, and what is left. */}
                {isToday
                  ? [
                      t.log.add[slotAt(now, timeZone)],
                      ...(day.data.remaining && day.data.remaining.kcal > 0
                        ? [t.log.left(f.number(day.data.remaining.kcal))]
                        : []),
                    ].join(' · ')
                  : t.log.open}
              </Button>
            )}
          </section>
        </div>
        <div className="space-y-4 lg:col-span-4">
          {/* Its fields start again with each day. */}
          <CheckIn key={`check-in ${day.data.date}`} day={day.data} future={future} />
          <BodyToday
            key={`body ${day.data.date}`}
            date={day.data.date}
            metrics={day.data.metrics}
            canLog={!future}
          />
        </div>
      </div>
      {!future && (
        <QuickAdd
          // Each day's meal of its own.
          key={`add ${day.data.date}`}
          open={adding !== undefined}
          onOpenChange={(open) => {
            if (!open) {
              setAdding(undefined)
            }
          }}
          initial={adding?.food}
          opener={opener}
          remaining={day.data.remaining?.kcal}
          date={day.data.date}
          today={isToday}
          timeZone={timeZone}
          startHour={session.data?.user.day_start_hour ?? 0}
        />
      )}
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
