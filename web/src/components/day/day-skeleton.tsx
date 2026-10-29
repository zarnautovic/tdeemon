import { mealColumns } from '@/components/day/meal-columns'
import { Skeleton } from '@/components/progress/skeleton'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// The seven days of a week, and the three meals the skeleton has.
const week = [0, 1, 2, 3, 4, 5, 6]
const meals = [0, 1, 2]

// DaySkeleton stands in for a day under its header while what it shows
// comes (DayPage), in its shape: on a phone the week above the hero, its
// ring, numbers and macros, and three meals' rows; on a desktop
// (columns) the hero and the meals' table, and beside them the week, the
// body and the check-in. A screen reader hears that it loads.
export function DaySkeleton({ columns }: { columns: boolean }) {
  const t = useMessages()

  return (
    <div aria-busy className="space-y-4">
      <output className="sr-only">{t.loading}</output>
      {!columns && (
        <div className="space-y-2">
          <WeekDays />
          <Skeleton className="h-11" />
        </div>
      )}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12 lg:items-start">
        <div className="space-y-4 lg:col-span-8">
          <Hero columns={columns} />
          <div className="space-y-2">
            <Skeleton className="h-6.5 w-24" />
            {columns ? <MealTable /> : <MealCards />}
          </div>
        </div>
        {columns && (
          <div className="space-y-4 lg:col-span-4">
            <div className="space-y-3 rounded-lg border bg-card p-4">
              <Skeleton className="h-5.5 w-28" />
              <WeekDays bars />
              <Skeleton className="h-11" />
            </div>
            <div className="space-y-3 rounded-lg border bg-card p-4">
              <Skeleton className="h-5.5 w-28" />
              <div className="grid grid-cols-2 gap-2">
                <Skeleton className="h-18" />
                <Skeleton className="h-18" />
              </div>
            </div>
            <div className="space-y-3 rounded-lg border bg-card p-4">
              <Skeleton className="h-5.5 w-24" />
              <Skeleton className="h-11" />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// WeekDays are the week's days: each a weekday, its mini ring on a phone
// or its bar on a desktop, and its number.
function WeekDays({ bars = false }: { bars?: boolean }) {
  return (
    <div className="grid grid-cols-7 gap-1">
      {week.map((d) => (
        <div key={d} className="flex flex-col items-center gap-1 py-1.5">
          <Skeleton className="h-4 w-6" />
          <Skeleton className={bars ? 'h-20 w-6 rounded-sm' : 'size-7 rounded-full'} />
          <Skeleton className="h-4 w-4" />
        </div>
      ))}
    </div>
  )
}

// Hero is the day's hero: the ring and the numbers beside it, in two
// columns on a phone and side by side on a desktop, the macros' bars,
// and on a phone the phase's line.
function Hero({ columns }: { columns: boolean }) {
  return (
    <div className="space-y-4 rounded-lg border bg-card p-4 lg:p-6">
      <div className="flex items-center gap-4 lg:gap-8">
        <Skeleton className="size-34 shrink-0 rounded-full" />
        <div className="grid max-w-72 flex-1 grid-cols-[1fr_auto] gap-x-3 gap-y-2 lg:max-w-none lg:grid-cols-4 lg:gap-x-6">
          {[0, 1, 2, 3].map((i) =>
            columns ? (
              <div key={i} className="space-y-1">
                <Skeleton className="h-4.5 w-16" />
                <Skeleton className="h-6.5 w-20" />
              </div>
            ) : (
              <div key={i} className="contents">
                <Skeleton className="h-4.5 w-20" />
                <Skeleton className="h-4.5 w-12" />
              </div>
            ),
          )}
        </div>
      </div>
      <div className="space-y-3 lg:grid lg:grid-cols-3 lg:gap-6 lg:space-y-0">
        {[0, 1, 2].map((i) => (
          <div key={i} className="space-y-1.5">
            <Skeleton className="h-4.5 w-28" />
            <Skeleton className="h-2 rounded-full" />
          </div>
        ))}
      </div>
      {!columns && <Skeleton className="h-9.5" />}
    </div>
  )
}

// MealCards are a phone's meals: cards of a row each, the time, the name
// and the kcal, the macros under them.
function MealCards() {
  const f = useFormat()

  return (
    <div className="space-y-2">
      {meals.map((m) => (
        <div
          key={m}
          className={cn(
            'grid items-center gap-x-2.5 gap-y-1.5 rounded-lg border bg-card px-4 py-3.5',
            f.hour12
              ? 'grid-cols-[3.75rem_minmax(0,1fr)_auto]'
              : 'grid-cols-[2.5rem_minmax(0,1fr)_auto]',
          )}
        >
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-5.5 w-3/5" />
          <Skeleton className="h-5.5 w-16" />
          <Skeleton className="col-span-2 col-start-2 h-4 w-4/5" />
        </div>
      ))}
    </div>
  )
}

// MealTable is a desktop's meals, a table: its heads, a row a meal, and
// the day's total.
function MealTable() {
  const f = useFormat()
  const grid = cn('grid items-center', mealColumns(f.hour12))

  return (
    <div className="rounded-lg border bg-card">
      <div className={cn(grid, 'border-b py-2')}>
        <Skeleton className="col-span-2 h-4 w-20" />
      </div>
      {meals.map((m) => (
        <div key={m} className={cn(grid, 'min-h-14 border-b py-3')}>
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-5.5 w-3/5" />
          <Skeleton className="h-5.5 w-full" />
          <Skeleton className="h-4.5 w-full" />
          <Skeleton className="h-4.5 w-full" />
          <Skeleton className="h-4.5 w-full" />
        </div>
      ))}
      <div className={cn(grid, 'py-3')}>
        <Skeleton className="col-span-2 h-5.5 w-16" />
        <Skeleton className="h-5.5 w-full" />
      </div>
    </div>
  )
}
