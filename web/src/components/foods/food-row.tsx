import { Link } from '@tanstack/react-router'

import type { Food } from '@/api/types'
import { MacroSplit } from '@/components/day/macro-split'
import { daysSinceEaten, eatenThisWeek } from '@/components/foods/eaten'
import { FoodAvatar } from '@/components/foods/food-avatar'
import { Badge } from '@/components/ui/badge'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import type { FoodsSearch } from '@/router'

// FoodRow is a food in a list: its initials, in colour when it was eaten
// this week and grey when not, its name and brand, whether its values are
// an estimate, its macros as a bar and in grams, how long ago it was
// eaten when not this week, and its kcal for its basis (/100 g); a link
// to the food, marked while it is the one open beside the list.
export function FoodRow({
  food,
  from,
  today,
}: {
  food: Food
  // The list the row is in, which the food's page keeps beside it, or
  // goes back to.
  from: FoodsSearch
  // The user's day, which this week is of; undefined until it is known.
  today: string | undefined
}) {
  const t = useMessages()
  const f = useFormat()
  const n = food.nutrients
  const recent = eatenThisWeek(food, today)
  const days = daysSinceEaten(food, today)

  return (
    <li>
      <Link
        to="/foods/$id"
        params={{ id: food.id }}
        search={from}
        // Open whatever the list beside it, and read or edited.
        activeOptions={{ includeSearch: false }}
        className="group flex min-h-16 items-center gap-3 px-3 py-2.5 outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset data-[status=active]:bg-primary-soft lg:px-4"
      >
        <FoodAvatar
          name={food.name}
          muted={!recent}
          // Apart from the row's colour, when it is the one open.
          className="group-data-[status=active]:bg-card"
        />
        <span className="min-w-0 flex-1 space-y-1">
          <span className="flex min-w-0 items-center gap-1.5">
            <span className="truncate font-medium">
              {food.name}
              {food.brand && (
                <span className="font-normal text-muted-foreground"> · {food.brand}</span>
              )}
            </span>
            {food.source === 'estimate' && <Badge tone="warning">{t.meal.estimate}</Badge>}
          </span>
          <span className="flex min-w-0 items-center gap-2 text-caption text-muted-foreground">
            <MacroSplit nutrients={n} />
            <span className="truncate">
              {t.day.short.protein} {f.grams(n.protein)} · {t.day.short.carbs} {f.grams(n.carbs)} ·{' '}
              {t.day.short.fat} {f.grams(n.fat)}
              {!recent && days !== undefined && ` · ${t.foods.eatenAgo(f.number(days), days)}`}
            </span>
          </span>
        </span>
        <span className="shrink-0 text-right">
          <span className="block font-semibold whitespace-nowrap">
            {t.unit.kcal(f.number(n.kcal))}
          </span>
          <span className="block text-caption text-muted-foreground">
            {t.foods.perShort[food.basis]}
          </span>
        </span>
      </Link>
    </li>
  )
}
