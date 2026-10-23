import type { Item, Meal } from '@/api/types'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// MealCard is a meal: when it was eaten and what, its kcal, and each item
// with how much of it and what it brought.
export function MealCard({ meal, timeZone }: { meal: Meal; timeZone: string }) {
  const t = useMessages()
  const f = useFormat()

  // amount is how much of an item, in its unit.
  function amount(item: Item): string {
    if (item.unit === 'portion') {
      return t.day.portions(f.number(item.amount, 2), item.amount)
    }
    return `${f.number(item.amount)} ${item.unit}`
  }

  return (
    <article className="space-y-2 rounded-xl border p-4">
      <header className="flex items-baseline justify-between gap-2">
        <h3 className="font-medium">
          <time dateTime={meal.eaten_at} className="text-muted-foreground tabular-nums">
            {f.time(meal.eaten_at, timeZone)}
          </time>{' '}
          {meal.description}
        </h3>
        <p className="font-semibold tabular-nums">{f.number(meal.total.kcal)} kcal</p>
      </header>
      <ul className="divide-y text-sm">
        {meal.items.map((item) => (
          <li key={item.id} className="flex items-baseline justify-between gap-2 py-1.5">
            <span>
              {item.name} <span className="text-muted-foreground">{amount(item)}</span>
            </span>
            <span className="text-right text-muted-foreground tabular-nums">
              {f.number(item.nutrients.kcal)} kcal · {t.day.short.protein}{' '}
              {f.number(item.nutrients.protein)} · {t.day.short.carbs}{' '}
              {f.number(item.nutrients.carbs)} · {t.day.short.fat} {f.number(item.nutrients.fat)}
            </span>
          </li>
        ))}
      </ul>
    </article>
  )
}
