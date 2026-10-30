import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { ChevronLeft, ChevronRight, Plus } from 'lucide-react'
import { use, useEffect, useRef, useState } from 'react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import type { Food } from '@/api/types'
import { BodyToday } from '@/components/day/body-today'
import { ChangesButton } from '@/components/day/changes-button'
import { CheckIn } from '@/components/day/check-in'
import { DayPhase } from '@/components/day/day-phase'
import { useDayReady } from '@/components/day/day-ready'
import { DaySkeleton } from '@/components/day/day-skeleton'
import { DaySummary } from '@/components/day/day-summary'
import { DayTypeChip } from '@/components/day/day-type-chip'
import { FirstSteps } from '@/components/day/first-steps'
import { kcalLeft } from '@/components/day/left'
import { MealList } from '@/components/day/meal-list'
import { slotAt } from '@/components/day/meal-slot'
import { MorningLine, MorningSummary } from '@/components/day/morning-summary'
import { QuickAdd } from '@/components/day/quick-add'
import { QuickChips } from '@/components/day/quick-chips'
import { WeekCard } from '@/components/day/week-card'
import { WeekLine } from '@/components/day/week-line'
import { WeekStrip } from '@/components/day/week-strip'
import { LoadError } from '@/components/load-error'
import { ProposalNudge } from '@/components/plan/proposal-nudge'
import { Skeleton } from '@/components/progress/skeleton'
import { Button, buttonVariants } from '@/components/ui/button'
import { addDays, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { LogHereContext } from '@/lib/log-here'
import { useDesktop } from '@/lib/media'
import { useShortcuts } from '@/lib/shortcuts'
import { dateAt } from '@/lib/zoned'
import type { DaySearch } from '@/router'

// DayKeys moves a day back or ahead by the keyboard's arrows, as the
// page's arrows do: to the day before, from the first day on, and to the
// day after up to today.
function DayKeys({
  before,
  after,
}: {
  before: DaySearch | undefined
  after: DaySearch | undefined
}) {
  const navigate = useNavigate()
  useShortcuts({
    ...(before && {
      ArrowLeft: () => {
        void navigate({ to: '/', search: before })
      },
    }),
    ...(after && {
      ArrowRight: () => {
        void navigate({ to: '/', search: after })
      },
    }),
  })
  return null
}

// DayPage is a day of the user's: today at /, another at /?date=YYYY-MM-DD.
// It shows the week's days and their average, what was eaten against the
// target, the meals, the check-in and the measurements (on a desktop in
// two columns, the meals a table), takes the day's type, the check-in
// and the measurements, logs a meal (QuickAdd), and moves a day back or
// ahead, up to today. /?log=true opens the meal to log on today (the
// app's Log a meal); /?summary=YYYY-MM-DD that morning's summary, which
// the morning's notification opens, and today's header a tap away.
export function DayPage() {
  const t = useMessages()
  const f = useFormat()
  const { date, log, summary } = useSearch({ from: '/app/' })
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
  // A desktop lays the day out in columns, beside its week.
  const desktop = useDesktop()
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const today = me.data?.today
  // When the page opened, for the meal of the hour to add.
  const [now] = useState(Date.now)
  // The meal being logged, open, and the food it starts with; and what
  // opened it, which the focus goes back to.
  const [adding, setAdding] = useState<{ food?: Food } | undefined>(undefined)
  const opener = useRef<HTMLElement | null>(null)
  // The meals' heading, which a meal deleted in its editor leaves the
  // focus to.
  const mealsHeading = useRef<HTMLHeadingElement>(null)
  // The line that opens this morning's summary, which its sheet gives the
  // focus back to.
  const morningOpener = useRef<HTMLButtonElement>(null)
  // A meal just logged or repeated, whose row takes the focus once it
  // shows, and a screen reader says it: at once, or as the quick add's
  // sheet closes, which gives the focus back to its opener.
  const [arrived, setArrived] = useState<string | undefined>(undefined)
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
  // Today's page logs the app's Log a meal itself, while it shows.
  const setLogHere = use(LogHereContext)
  useEffect(() => {
    if (setLogHere === null || date !== undefined) {
      return undefined
    }
    setLogHere((from) => {
      opener.current = from
      setAdding({})
    })
    return () => {
      setLogHere(null)
    }
  }, [setLogHere, date])

  // While another day comes, the one before stays on the page
  // (placeholder data), and the title and arrows already follow the day
  // asked for.
  const waiting = day.isPlaceholderData
  // What the day shows above its meals, come with it: the page shows it
  // all at once, under its header, and the day's skeleton until then;
  // the first time, as another day keeps the one before until it comes.
  const ready = useDayReady(date, waiting ? undefined : day.data)
  const [painted, setPainted] = useState(false)
  if (ready && day.data !== undefined && !painted) {
    setPainted(true)
  }
  // Today is the day / answered, once it has: /me's may be older, the
  // day's start hour having passed since.
  const todayDate = date === undefined && !waiting && day.data !== undefined ? day.data.date : today
  // The day shown, unknown only while neither the day nor the user came.
  const shown = date ?? todayDate ?? day.data?.date
  const isToday = date === undefined || shown === todayDate
  // A day to come, which an address may name though the arrows stop at
  // today, takes its type and exclusion only, as the server has it. A day
  // an address names may be one until the app knows today; if it cannot
  // learn it, the server says what it refuses.
  const future =
    shown !== undefined && todayDate !== undefined
      ? shown > todayDate
      : date !== undefined && me.isPending
  // A day's link: today's is / itself.
  const searchOf = (d: string) => (d === todayDate ? {} : { date: d })
  // The arrows go back to the first day a history is of, which the
  // router takes a day from, and ahead up to today.
  const hasBefore = shown !== undefined && shown > limits.history.since
  const hasNext = shown !== undefined && todayDate !== undefined && shown < todayDate
  const before = hasBefore ? addDays(shown, -1) : undefined
  const after = hasNext ? addDays(shown, 1) : undefined
  const arrow = buttonVariants({ variant: 'ghost', size: 'icon-sm' })
  // An arrow there is none of keeps the place of one.
  const noArrow = <span className="size-11 shrink-0 lg:size-9" />
  const previous =
    before !== undefined ? (
      <Link
        to="/"
        search={searchOf(before)}
        // Another day is never the current page: its date must match too,
        // not be left out.
        activeOptions={{ exact: true }}
        aria-label={t.day.previous}
        className={arrow}
      >
        <ChevronLeft aria-hidden />
      </Link>
    ) : (
      noArrow
    )
  const next =
    after !== undefined ? (
      <Link
        to="/"
        search={searchOf(after)}
        activeOptions={{ exact: true }}
        aria-label={t.day.next}
        className={arrow}
      >
        <ChevronRight aria-hidden />
      </Link>
    ) : (
      noArrow
    )
  // Of the day shown, not of the one before while it comes; its place
  // kept while the day comes. Beside it the recent changes, whose Undo
  // reaches past a toast.
  let dayType
  if (day.data !== undefined) {
    dayType = (
      <div inert={waiting} className="flex items-center gap-1">
        <DayTypeChip key={`type ${day.data.date}`} day={day.data} />
        <ChangesButton />
      </div>
    )
  } else if (day.isPending) {
    dayType = <Skeleton className="h-9 w-28 rounded-full" />
  }
  // The day's date, its place kept while it is not known.
  const dated = shown === undefined ? <Skeleton className="h-4.5 w-36" /> : f.date(shown, todayDate)

  const toToday = (
    <Link
      to="/"
      activeOptions={{ exact: true }}
      className="relative flex h-9 shrink-0 items-center rounded-full bg-primary-soft px-4 text-small font-semibold text-primary outline-none after:absolute after:inset-x-0 after:-inset-y-1 after:content-[''] hover:bg-primary-soft/70 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
    >
      {t.day.toToday}
    </Link>
  )
  let header
  if (isToday) {
    // Today's: the day's type beside the title, and the date under both
    // on a line of its own, which a phone's width keeps whole.
    header = (
      <header className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2">
        <div className="row-span-2 flex shrink-0 items-center">
          {previous}
          {next}
        </div>
        <h1 className="text-title">{t.day.today}</h1>
        {shown === undefined ? (
          <div className="col-span-2 col-start-2 row-start-2">{dated}</div>
        ) : (
          <p className="col-span-2 col-start-2 row-start-2 text-small text-muted-foreground">
            {dated}
          </p>
        )}
        <div className="col-start-3 row-start-1">{dayType}</div>
      </header>
    )
  } else if (desktop) {
    // Another day's on a desktop, as today's is: its date the title, and
    // its type and the way back to today at the other end.
    header = (
      <header className="flex items-center gap-2">
        <div className="flex shrink-0 items-center">
          {previous}
          {next}
        </div>
        <h1 className="min-w-0 flex-1 text-title">{dated}</h1>
        {dayType}
        {toToday}
      </header>
    )
  } else {
    // Another day's on a phone: its date between the arrows, on a line
    // of its own, its type under it, and the way back to today right
    // there.
    header = (
      <header className="flex items-start gap-2">
        {previous}
        <div className="flex min-w-0 flex-1 flex-col items-center gap-2 text-center">
          <h1 className="flex min-h-11 items-center text-h3 text-balance">{dated}</h1>
          {dayType}
          {toToday}
        </div>
        {next}
      </header>
    )
  }
  // The keys, and the morning summary asked for by the address, over
  // the day whether it came or not.
  const keys = (
    <>
      <DayKeys
        before={before === undefined ? undefined : searchOf(before)}
        after={after === undefined ? undefined : searchOf(after)}
      />
      <MorningSummary
        date={summary}
        finalFocus={morningOpener}
        onClose={() => {
          void navigate({ to: '/', search: (s) => ({ ...s, summary: undefined }), replace: true })
        }}
      />
    </>
  )
  // Until the day comes, its skeleton under the header; or why it did
  // not come, with a way to ask again.
  if (day.data === undefined) {
    return (
      <div className="max-w-3xl space-y-4 lg:max-w-none">
        {keys}
        {header}
        {day.isError ? (
          <LoadError
            title={isToday ? t.loadFailed.today : t.loadFailed.day}
            error={day.error}
            onRetry={() => void day.refetch()}
          />
        ) : (
          <DaySkeleton columns={desktop} />
        )}
      </div>
    )
  }
  // Until what it shows above its meals comes too, its parts are there
  // but hidden, asking for it, and the skeleton stays.
  const hide = !(ready || painted)

  // What the day's target leaves, as its ring says it.
  const left =
    day.data.target === undefined ? undefined : kcalLeft(day.data.total.kcal, day.data.target.kcal)
  // What the user says of the day, and what it measured: their fields
  // start again with each day.
  const checkIn = <CheckIn key={`check-in ${day.data.date}`} day={day.data} future={future} />
  const body = (
    <BodyToday
      key={`body ${day.data.date}`}
      date={day.data.date}
      today={todayDate}
      metrics={day.data.metrics}
      canLog={!future}
    />
  )

  return (
    <div className="max-w-3xl space-y-4 lg:max-w-none">
      {keys}
      {header}
      {hide && <DaySkeleton columns={desktop} />}
      {/* A phone's week, above the day; a desktop's is beside it. */}
      {!desktop && shown !== undefined && todayDate !== undefined && (
        <div hidden={hide} className="space-y-2">
          <WeekStrip date={shown} today={todayDate} />
          <WeekLine date={shown} today={todayDate} />
        </div>
      )}
      <div
        hidden={hide}
        aria-busy={waiting}
        // What is changed while it waits would be the day before's.
        inert={waiting}
        className={`grid grid-cols-1 gap-4 lg:grid-cols-12 lg:items-start ${waiting ? 'opacity-60' : ''}`}
      >
        {/* A desktop's columns: the day and its meals, a table, and
            beside them the week, the body, the check-in and the phase. A
            phone has them one after the other, the phase in the day's
            hero. */}
        <div className="space-y-4 lg:col-span-8">
          {/* A new user's first steps, in place of the hero before a plan. */}
          <FirstSteps day={day.data}>
            <DaySummary day={day.data} phase={!desktop} />
          </FirstSteps>
          {/* A proposal that waits, under the day's hero. */}
          {isToday && <ProposalNudge />}
          {/* This morning's summary: of the calendar's date, as its
              notification's, not the day's, which may still be yesterday
              for a day that starts late. */}
          {isToday && (
            <MorningLine
              ref={morningOpener}
              onOpen={() => {
                void navigate({ to: '/', search: { summary: dateAt(Date.now(), timeZone) } })
              }}
            />
          )}
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
              <h2 id="day-meals" ref={mealsHeading} tabIndex={-1} className="text-h2 outline-none">
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
              <MealList
                day={day.data}
                today={todayDate ?? day.data.date}
                timeZone={timeZone}
                startHour={session.data?.user.day_start_hour ?? 0}
                afterDelete={mealsHeading}
                columns={desktop}
                arrived={arrived}
                onArrived={(row) => {
                  setArrived(undefined)
                  opener.current = row
                  if (adding === undefined) {
                    row.focus()
                  }
                }}
                onRepeated={setArrived}
              />
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
                      ...(left !== undefined && left > 0 ? [t.log.left(f.number(left))] : []),
                    ].join(' · ')
                  : t.log.open}
              </Button>
            )}
          </section>
        </div>
        <div className="space-y-4 lg:col-span-4">
          {desktop ? (
            <>
              {shown !== undefined && todayDate !== undefined && (
                <WeekCard date={shown} today={todayDate} />
              )}
              {body}
              {checkIn}
              <DayPhase date={day.data.date} card />
            </>
          ) : (
            <>
              {checkIn}
              {body}
            </>
          )}
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
          onLogged={(meal) => {
            setArrived(meal.id)
          }}
          remaining={left}
          date={day.data.date}
          today={isToday}
          timeZone={timeZone}
          startHour={session.data?.user.day_start_hour ?? 0}
        />
      )}
    </div>
  )
}
