import { useId, useState } from 'react'

import { useFindFoods } from '@/api/foods'
import type { Food } from '@/api/types'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// FoodPicker finds a food of the user's to add to a meal: what a search
// finds, each a button that picks it. label names its search, when it is
// not to add a food.
export function FoodPicker({ onPick, label }: { onPick: (food: Food) => void; label?: string }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const [query, setQuery] = useState('')
  const found = useFindFoods(query)
  const searching = query.trim() !== ''

  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className={cn(fieldLabel, 'block')}>
        {label ?? t.log.find}
      </label>
      <input
        id={id}
        type="search"
        value={query}
        placeholder={t.log.findHint}
        autoComplete="off"
        className={inputClass}
        onChange={(event) => {
          setQuery(event.currentTarget.value)
        }}
        onKeyDown={(event) => {
          // Enter picks the first food found; it does not send the meal.
          if (event.key === 'Enter') {
            event.preventDefault()
            const firstFound = found.data?.foods[0]
            if (searching && firstFound) {
              onPick(firstFound)
              setQuery('')
            }
          }
        }}
      />
      {searching && found.isSuccess && found.data.foods.length === 0 && (
        <p className="text-sm text-muted-foreground">{t.log.noFood}</p>
      )}
      {searching && found.isSuccess && found.data.foods.length > 0 && (
        <ul aria-label={t.foods.found} className="divide-y rounded-lg border">
          {found.data.foods.map((food) => (
            <li key={food.id}>
              <button
                type="button"
                className="flex w-full items-baseline justify-between gap-2 px-3 py-2 text-left outline-none hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50"
                onClick={() => {
                  onPick(food)
                  setQuery('')
                }}
              >
                <span>
                  {food.name}
                  {food.brand && <span className="text-muted-foreground"> · {food.brand}</span>}
                </span>
                <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                  {t.unit.kcal(f.number(food.nutrients.kcal))} {t.foods.per[food.basis]}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
