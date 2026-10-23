import { Pencil } from 'lucide-react'
import { useState } from 'react'

import type { Item, Meal, Warning } from '@/api/types'
import { MealEditor } from '@/components/day/meal-editor'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// MealCard is a meal: when it was eaten and what, its kcal, and each item
// with how much of it and what it brought. It can be changed (MealEditor),
// and says what the server warned of the last change.
export function MealCard({ meal, date, timeZone }: { meal: Meal; date: string; timeZone: string }) {
  const t = useMessages()
  const f = useFormat()
  const [editing, setEditing] = useState(false)
  const [warnings, setWarnings] = useState<Warning[]>([])
  const time = f.time(meal.eaten_at, timeZone)

  // amount is how much of an item, in its unit.
  function amount(item: Item): string {
    if (item.unit === 'portion') {
      // The word's form follows the number shown: 0.999 is "1 portion".
      const shown = Math.round(item.amount * 100) / 100
      return t.day.portions(f.number(shown, 2), shown)
    }
    return t.unit[item.unit](f.number(item.amount))
  }

  return (
    <article className="space-y-2 rounded-xl border p-4">
      <header className="flex items-baseline justify-between gap-2">
        <h3 className="font-medium">
          <time dateTime={meal.eaten_at} className="text-muted-foreground tabular-nums">
            {time}
          </time>{' '}
          {meal.description}
        </h3>
        <div className="flex items-baseline gap-1">
          <p className="font-semibold tabular-nums">{t.unit.kcal(f.number(meal.total.kcal))}</p>
          {!editing && (
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t.meal.edit(meal.description || time)}
              className="self-center"
              onClick={() => {
                setEditing(true)
                setWarnings([])
              }}
            >
              <Pencil aria-hidden />
            </Button>
          )}
        </div>
      </header>
      {editing ? (
        <MealEditor
          meal={meal}
          date={date}
          onDone={(w) => {
            setEditing(false)
            setWarnings(w)
          }}
        />
      ) : (
        <ul className="divide-y text-sm">
          {meal.items.map((item) => (
            <li key={item.id} className="flex items-baseline justify-between gap-2 py-1.5">
              <span>
                {item.name} <span className="text-muted-foreground">{amount(item)}</span>
              </span>
              <span className="text-right text-muted-foreground tabular-nums">
                {t.unit.kcal(f.number(item.nutrients.kcal))} · {t.day.short.protein}{' '}
                {f.number(item.nutrients.protein)} · {t.day.short.carbs}{' '}
                {f.number(item.nutrients.carbs)} · {t.day.short.fat} {f.number(item.nutrients.fat)}
              </span>
            </li>
          ))}
        </ul>
      )}
      {warnings.length > 0 && (
        <output className="block text-sm text-amber-700 dark:text-amber-400">
          <ul className="space-y-1">
            {warnings.map((w) => (
              <li key={w.code}>{t.meal.warnings[w.code]}</li>
            ))}
          </ul>
        </output>
      )}
    </article>
  )
}
