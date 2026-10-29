import { Repeat2 } from 'lucide-react'

import { $api } from '@/api/client'
import { useFrequentFoods } from '@/api/foods'
import { useLogMeal, useRecentMeals } from '@/api/meals'
import type { Food } from '@/api/types'
import { lastAmountOf, repeatOf } from '@/components/day/meal-entries'
import { mealName, slotAt } from '@/components/day/meal-slot'
import { RepeatStatus } from '@/components/day/repeat-meal'
import { suggestFoods } from '@/components/day/suggest'
import { Button } from '@/components/ui/button'
import { addDays, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { sayChange } from '@/lib/toast'

// How many of the foods logged most the chips offer.
const chipFoods = 4

// QuickChips add to today in a tap: yesterday's meal of the meal of the
// day it is now (Yesterday's lunch), logged again now, and the foods
// logged most, those eaten at that meal of the day lately first, as the
// quick add offers them (suggestFoods), each opening the meal to log with
// it in it, in the amount it was eaten in last (onFood). Nothing to
// offer, they are not there.
export function QuickChips({
  date,
  timeZone,
  now,
  onFood,
}: {
  // Today, YYYY-MM-DD.
  date: string
  timeZone: string
  // The instant whose meal of the day yesterday's is of.
  now: number
  // A food's chip, tapped: the food, and the chip.
  onFood: (food: Food, chip: HTMLElement) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const slot = slotAt(now, timeZone)
  const yesterday = $api.useQuery('get', '/api/v1/days/{date}', {
    params: { path: { date: addDays(date, -1) } },
  })
  // The list the quick add's Frequent shows: one query for both.
  const frequent = useFrequentFoods()
  const recent = useRecentMeals()
  const repeat = useLogMeal(date, (answer) => {
    sayChange(
      {
        title: t.meal.repeated(
          mealName(answer.meal, t),
          t.unit.kcal(f.number(answer.meal.total.kcal)),
        ),
        lines: answer.warnings.map((w) => t.meal.warnings[w.code]),
      },
      { last: { action: 'meal.create', entity: answer.meal.id }, date },
    )
  })
  const meal = yesterday.data?.meals.findLast(
    (m) => slotAt(Date.parse(m.eaten_at), timeZone) === slot,
  )
  const foods = suggestFoods(
    (frequent.data?.foods ?? []).filter((food) => food.last_use !== undefined),
    recent.data?.meals ?? [],
    { slot, now, timeZone, date },
  ).slice(0, chipFoods)

  if (meal === undefined && foods.length === 0) {
    return null
  }
  return (
    <section aria-labelledby="day-quick" className="space-y-2">
      <h2 id="day-quick" className="text-small font-semibold text-muted-foreground">
        {t.quick.title}
      </h2>
      <div className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1 lg:mx-0 lg:flex-wrap lg:px-0">
        {meal !== undefined && (
          <Button
            variant="outline"
            className="shrink-0"
            disabled={repeat.isPending}
            onClick={() => {
              repeat.mutate({ body: repeatOf(meal) })
            }}
          >
            <Repeat2 aria-hidden className="text-primary" />
            {t.quick.yesterday[slot]}
          </Button>
        )}
        {foods.map((food) => (
          <Button
            key={food.id}
            variant="outline"
            className="shrink-0"
            onClick={(e) => {
              onFood(food, e.currentTarget)
            }}
          >
            {[food.name, lastAmountOf(food, t, f)].join(' ')}
          </Button>
        ))}
      </div>
      <RepeatStatus log={repeat} />
    </section>
  )
}
