import { Link } from '@tanstack/react-router'

import type { Food } from '@/api/types'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import type { FoodsSearch } from '@/router'

// FoodRow is a food in a list: its name and brand, what it brings per its
// basis, and the day it was eaten last; a link to the food.
export function FoodRow({
  food,
  timeZone,
  from,
}: {
  food: Food
  timeZone: string
  // The foods page the row is on, which the food's page goes back to.
  from: FoodsSearch
}) {
  const t = useMessages()
  const f = useFormat()
  const n = food.nutrients

  return (
    <li>
      <Link
        to="/foods/$id"
        params={{ id: food.id }}
        state={{ foods: from }}
        className="flex items-baseline justify-between gap-3 rounded-lg px-2 py-2.5 outline-none hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <span className="min-w-0">
          <span className="font-medium">{food.name}</span>
          {food.brand && <span className="text-muted-foreground"> · {food.brand}</span>}
          {food.archived && (
            <span className="text-xs text-muted-foreground"> ({t.foods.archivedMark})</span>
          )}
          {food.last_use && (
            <span className="block text-xs text-muted-foreground">
              {t.foods.lastEaten(f.calendarDay(food.last_use.day, timeZone))}
            </span>
          )}
        </span>
        <span className="shrink-0 text-right text-sm text-muted-foreground tabular-nums">
          {t.unit.kcal(f.number(n.kcal))} · {t.day.short.protein} {f.number(n.protein)} ·{' '}
          {t.day.short.carbs} {f.number(n.carbs)} · {t.day.short.fat} {f.number(n.fat)}
          <span className="block text-xs">{t.foods.per[food.basis]}</span>
        </span>
      </Link>
    </li>
  )
}
