import { Pencil } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { useLogMeal } from '@/api/meals'
import type { Item, Meal, Warning } from '@/api/types'
import { MealEditor } from '@/components/day/meal-editor'
import { repeatOf } from '@/components/day/meal-entries'
import { RepeatButton, RepeatStatus } from '@/components/day/repeat-meal'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// MealCard is a meal: when it was eaten and what, its kcal, and each item
// with how much of it and what it brought. It can be changed (MealEditor),
// and says what the server warned of the last change; or be repeated,
// today at this time.
export function MealCard({
  meal,
  date,
  today,
  timeZone,
  startHour,
}: {
  meal: Meal
  date: string
  // The user's logical today, which a repeat is of.
  today: string
  // The user's clock: its time zone and the hour a day starts at.
  timeZone: string
  startHour: number
}) {
  const t = useMessages()
  const f = useFormat()
  // A repeat is today's, but goes in turn with the changes of the day
  // shown, the undo of its page among them: an undo tapped meanwhile
  // takes back the repeat, not the change before it.
  const repeat = useLogMeal(date)
  const [editing, setEditing] = useState(false)
  const [warnings, setWarnings] = useState<Warning[]>([])
  // The pencil, which takes the focus back when the editor closes.
  const pencil = useRef<HTMLButtonElement>(null)
  const [closed, setClosed] = useState(false)
  useEffect(() => {
    if (closed) {
      pencil.current?.focus()
    }
  }, [closed])
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
            <RepeatButton
              label={t.meal.repeat(meal.description || time)}
              pending={repeat.isPending}
              onClick={() => {
                repeat.mutate({ body: repeatOf(meal) })
              }}
            />
          )}
          {!editing && (
            <Button
              ref={pencil}
              variant="ghost"
              size="icon-sm"
              aria-label={t.meal.edit(meal.description || time)}
              className="self-center"
              onClick={() => {
                setEditing(true)
                setClosed(false)
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
          today={today}
          timeZone={timeZone}
          startHour={startHour}
          onDone={(w) => {
            setEditing(false)
            setClosed(true)
            setWarnings(w)
          }}
        />
      ) : (
        <>
          {meal.note && (
            <p className="text-sm whitespace-pre-line text-muted-foreground">{meal.note}</p>
          )}
          <ul className="divide-y text-sm">
            {meal.items.map((item) => (
              <li key={item.id} className="flex items-baseline justify-between gap-2 py-1.5">
                <span>
                  {item.name} <span className="text-muted-foreground">{amount(item)}</span>
                </span>
                <span className="text-right text-muted-foreground tabular-nums">
                  {t.unit.kcal(f.number(item.nutrients.kcal))} · {t.day.short.protein}{' '}
                  {f.number(item.nutrients.protein)} · {t.day.short.carbs}{' '}
                  {f.number(item.nutrients.carbs)} · {t.day.short.fat}{' '}
                  {f.number(item.nutrients.fat)}
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
      <RepeatStatus log={repeat} isToday={date === today} />
      {warnings.length > 0 && (
        <output className="block text-sm text-amber-700 dark:text-amber-400">
          <ul className="space-y-1">
            {warnings.map((w, i) => (
              // Two items may bring a warning of one code.
              <li key={`${w.code} ${String(i)}`}>{t.meal.warnings[w.code]}</li>
            ))}
          </ul>
        </output>
      )}
    </article>
  )
}
