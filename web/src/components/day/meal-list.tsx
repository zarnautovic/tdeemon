import { useEffect, useRef, useState, type RefObject } from 'react'

import type { Day } from '@/api/types'
import { MealCard, type RowMove } from '@/components/day/meal-card'
import { mealColumns } from '@/components/day/meal-columns'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

const macros = ['protein', 'carbs', 'fat'] as const

// MealList is a day's meals, each a row that opens to its items
// (MealCard): on a phone, cards one under the other; on a desktop
// (columns), a table, whose heads name its columns and whose last row is
// the day's total. Each row says its values to a screen reader with their
// names, so the heads are for the eye. The table is one stop of Tab, the
// row the keyboard was on last (the first, at first), and the arrows,
// Home and End move between its rows (a roving tabindex). A meal that
// arrived (just logged or repeated) has its row given to onArrived once
// it shows, to take the focus; a repeat of today's says its meal's id to
// onRepeated.
export function MealList({
  day,
  today,
  timeZone,
  startHour,
  afterDelete,
  columns = false,
  arrived,
  onArrived,
  onRepeated,
}: {
  day: Day
  // The user's logical today, which a repeat is of.
  today: string
  timeZone: string
  startHour: number
  afterDelete: RefObject<HTMLElement | null>
  columns?: boolean
  arrived?: string | undefined
  onArrived?: ((row: HTMLElement) => void) | undefined
  onRepeated?: ((id: string) => void) | undefined
}) {
  const t = useMessages()
  const f = useFormat()
  const list = useRef<HTMLUListElement>(null)
  // The meal whose row the keyboard was on last: Tab's stop in the table.
  const [active, setActive] = useState<string | undefined>(undefined)
  const current = day.meals.some((m) => m.id === active) ? active : day.meals[0]?.id

  // The meal that arrived, once the day has it.
  const shows = arrived !== undefined && day.meals.some((m) => m.id === arrived)
  useEffect(() => {
    const row = shows
      ? list.current?.querySelector<HTMLElement>(`[data-meal="${arrived}"]`)
      : undefined
    if (row) {
      onArrived?.(row)
    }
  }, [shows, arrived, onArrived])

  // move takes the focus from the row of meal from to another.
  function move(from: string, to: RowMove) {
    const rows = Array.from(list.current?.querySelectorAll<HTMLElement>('[data-meal]') ?? [])
    const at = rows.findIndex((r) => r.dataset.meal === from)
    const next = {
      previous: Math.max(0, at - 1),
      next: Math.min(rows.length - 1, at + 1),
      first: 0,
      last: rows.length - 1,
    }[to]
    rows[next]?.focus()
  }

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
        keys={
          columns
            ? {
                tabbable: meal.id === current,
                onFocus: () => {
                  setActive(meal.id)
                },
                onMove: (to) => {
                  move(meal.id, to)
                },
              }
            : undefined
        }
        onRepeated={onRepeated}
      />
    </li>
  ))
  if (!columns) {
    return (
      <ul ref={list} className="space-y-2">
        {rows}
      </ul>
    )
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
      <ul ref={list} className="divide-y">
        {rows}
      </ul>
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
