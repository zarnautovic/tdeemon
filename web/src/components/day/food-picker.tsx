import { useId, useState, type RefObject } from 'react'

import { useFindFoods } from '@/api/foods'
import type { Food } from '@/api/types'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// FoodPicker finds a food of the user's to add to a meal: what a search
// finds, each a button that picks it. label names its search, when it is
// not to add a food; onSearch hears of what is typed in it; field is given
// its search's field, for the focus. onCancel, when given, is a Cancel
// beside the field, for a search not to go on with.
export function FoodPicker({
  onPick,
  onSearch,
  onCancel,
  label,
  field,
}: {
  onPick: (food: Food) => void
  onSearch?: () => void
  onCancel?: () => void
  label?: string
  field?: RefObject<HTMLInputElement | null>
}) {
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
      <div className="flex gap-2">
        <input
          ref={field}
          id={id}
          type="search"
          value={query}
          placeholder={t.log.findHint}
          autoComplete="off"
          className={cn(inputClass, 'min-w-0 flex-1')}
          onChange={(event) => {
            setQuery(event.currentTarget.value)
            onSearch?.()
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
        {onCancel && (
          <Button variant="ghost" onClick={onCancel}>
            {t.log.cancel}
          </Button>
        )}
      </div>
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
