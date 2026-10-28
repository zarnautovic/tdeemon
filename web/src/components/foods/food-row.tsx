import { Link } from '@tanstack/react-router'

import type { Food } from '@/api/types'
import { MacroSplit } from '@/components/day/macro-split'
import { FoodAvatar } from '@/components/foods/food-avatar'
import { Badge } from '@/components/ui/badge'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import type { FoodsSearch } from '@/router'

// FoodRow is a food in a list: its initials, its name and brand, whether
// its values are an estimate, its macros as a bar and in grams, and its
// kcal for its basis (/100 g); a link to the food, marked while it is
// the one open beside the list.
export function FoodRow({
  food,
  from,
}: {
  food: Food
  // The list the row is in, which the food's page keeps beside it, or
  // goes back to.
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
        search={from}
        // Open whatever the list beside it, and read or edited.
        activeOptions={{ includeSearch: false }}
        className="flex min-h-16 items-center gap-3 px-3 py-2.5 outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset data-[status=active]:bg-primary-soft lg:px-4"
      >
        <FoodAvatar name={food.name} />
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
