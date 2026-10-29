import type { RefObject } from 'react'

import type { Day } from '@/api/types'
import { MealCard } from '@/components/day/meal-card'
import { mealColumns } from '@/components/day/meal-columns'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

const macros = ['protein', 'carbs', 'fat'] as const

// MealList is a day's meals, each a row that opens to its items
// (MealCard): on a phone, cards one under the other; on a desktop
// (columns), a table, whose heads name its columns and whose last row is
// the day's total. Each row says its values to a screen reader with their
// names, so the heads are for the eye.
export function MealList({
  day,
  today,
  timeZone,
  startHour,
  afterDelete,
  columns = false,
}: {
  day: Day
  // The user's logical today, which a repeat is of.
  today: string
  timeZone: string
  startHour: number
  afterDelete: RefObject<HTMLElement | null>
  columns?: boolean
}) {
  const t = useMessages()
  const f = useFormat()
  const rows = day.meals.map((meal) => (
    <li key={meal.id}>
      <MealCard
        meal={meal}
        date={day.date}
        today={today}
        timeZone={timeZone}
        startHour={startHour}
        afterDelete={afterDelete}
        columns={columns}
      />
    </li>
  ))
  if (!columns) {
    return <ul className="space-y-2">{rows}</ul>
  }
  const grid = cn('grid items-baseline', mealColumns(f.hour12))
  const c = t.day.columns
  const kcal = f.number(day.total.kcal)

  return (
    <div className="rounded-lg border bg-card">
      <div aria-hidden className={cn(grid, 'border-b py-2 text-caption text-muted-foreground')}>
        <span>{c.time}</span>
        <span>{c.meal}</span>
        <span className="text-right">{c.kcal}</span>
        {macros.map((m) => (
          <span key={m} className="text-right">
            {t.day.short[m]}
          </span>
        ))}
      </div>
      <ul className="divide-y">{rows}</ul>
      {/* The day's total, as its summary has it. */}
      <p className={cn(grid, 'border-t py-3 font-semibold')}>
        <span className="col-span-2">{c.total}</span>
        <span className="text-right whitespace-nowrap tabular-nums">
          <span aria-hidden>{kcal}</span>
          <span className="sr-only">{t.unit.kcal(kcal)}</span>
        </span>
        {macros.map((m) => (
          <span key={m} className="text-right text-small tabular-nums">
            <span className="sr-only">{t.day.short[m]} </span>
            {f.grams(day.total[m])}
          </span>
        ))}
      </p>
    </div>
  )
}
