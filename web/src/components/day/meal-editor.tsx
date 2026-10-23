import { RotateCcw, X } from 'lucide-react'
import { useState, type SubmitEvent } from 'react'

import { statusOf } from '@/api/errors'
import { useDeleteMeal, useUpdateMeal } from '@/api/meals'
import type { components } from '@/api/schema'
import type { Item, Meal, Warning } from '@/api/types'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

type Op = components['schemas']['MealOp']
type ItemInput = components['schemas']['ItemInput']

// An amount is how much of an item was given, as it was given (grams,
// portions, servings), and the item that puts another number in its
// place. Only an item of a food has one: an estimate is of a whole plate.
interface Amount {
  n: number
  unit: string | undefined
  with: (n: number) => ItemInput
}

function amountOf(item: Item, portions: string): Amount | undefined {
  const foodId = item.food_id
  if (foodId === undefined) {
    return undefined
  }
  const q = item.quantity
  const base: ItemInput =
    item.note === undefined ? { food_id: foodId } : { food_id: foodId, note: item.note }
  if (q.servings !== undefined && q.serving !== undefined) {
    const serving = q.serving
    return { n: q.servings, unit: serving, with: (n) => ({ ...base, serving, servings: n }) }
  }
  if (q.portions !== undefined) {
    return { n: q.portions, unit: portions, with: (n) => ({ ...base, portions: n }) }
  }
  if (q.amount !== undefined) {
    // Left out, the unit is the food's.
    const unit = q.unit ?? (item.unit === 'portion' ? undefined : item.unit)
    return {
      n: q.amount,
      unit,
      with: (n) => (unit === undefined ? { ...base, amount: n } : { ...base, amount: n, unit }),
    }
  }
  return undefined
}

// MealEditor changes a meal's items, the amount of each of a food and
// which stay, in one change saved at once, or deletes the meal after
// asking. onDone gives the server's warnings of the change.
export function MealEditor({
  meal,
  date,
  onDone,
}: {
  meal: Meal
  date: string
  onDone: (warnings: Warning[]) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const update = useUpdateMeal(date)
  const remove = useDeleteMeal(date)
  // The items to remove, and those whose amount is not a number.
  const [removed, setRemoved] = useState<ReadonlySet<string>>(new Set())
  const [wrong, setWrong] = useState<ReadonlySet<string>>(new Set())
  const [confirming, setConfirming] = useState(false)
  const none = removed.size === meal.items.length
  // A change the server refuses (422, or 409 for an item changed since)
  // would be refused again.
  const refused = update.isError && [409, 422].includes(statusOf(update.error) ?? 0)

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const ops: Op[] = []
    const notNumbers = new Set<string>()
    for (const item of meal.items) {
      if (removed.has(item.id)) {
        ops.push({ op: 'remove', item_id: item.id })
        continue
      }
      const amount = amountOf(item, t.meal.portions)
      const typed = form.get(item.id)
      if (amount === undefined || typeof typed !== 'string') {
        continue
      }
      const n = f.parse(typed)
      if (n === undefined) {
        notNumbers.add(item.id)
      } else if (n !== amount.n) {
        ops.push({ op: 'replace', item_id: item.id, item: amount.with(n) })
      }
    }
    setWrong(notNumbers)
    if (notNumbers.size > 0) {
      return
    }
    if (ops.length === 0) {
      onDone([])
      return
    }
    update.mutate(
      { params: { path: { id: meal.id } }, body: { ops } },
      {
        onSuccess: (answer) => {
          onDone(answer.warnings)
        },
      },
    )
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-3">
      <ul className="divide-y text-sm">
        {meal.items.map((item) => {
          const amount = amountOf(item, t.meal.portions)
          const gone = removed.has(item.id)
          return (
            <li key={item.id} className="flex flex-wrap items-center gap-2 py-2">
              <span className={cn('min-w-0 flex-1', gone && 'text-muted-foreground line-through')}>
                {item.name}
                {gone && <span className="sr-only"> ({t.meal.removed})</span>}
              </span>
              {amount && !gone && (
                <span className="relative w-32">
                  <input
                    name={item.id}
                    aria-label={t.meal.amount(item.name)}
                    inputMode="decimal"
                    autoComplete="off"
                    defaultValue={f.field(amount.n)}
                    aria-invalid={wrong.has(item.id)}
                    className={cn(inputClass, 'h-9 tabular-nums', amount.unit && 'pr-14')}
                  />
                  {amount.unit && (
                    <span className="pointer-events-none absolute inset-y-0 right-2 flex max-w-12 items-center truncate text-xs text-muted-foreground">
                      {amount.unit}
                    </span>
                  )}
                </span>
              )}
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={gone ? t.meal.keep(item.name) : t.meal.remove(item.name)}
                onClick={() => {
                  const next = new Set(removed)
                  if (gone) {
                    next.delete(item.id)
                  } else {
                    next.add(item.id)
                  }
                  setRemoved(next)
                }}
              >
                {gone ? <RotateCcw aria-hidden /> : <X aria-hidden />}
              </Button>
            </li>
          )
        })}
      </ul>
      {wrong.size > 0 && (
        <p role="alert" className="text-xs text-destructive">
          {t.day.notANumber}
        </p>
      )}
      {none && <p className="text-xs text-muted-foreground">{t.meal.oneItem}</p>}
      {refused ? (
        <p role="alert" className="text-xs text-destructive">
          {t.meal.refused}
        </p>
      ) : (
        <SaveStatus change={update} />
      )}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={none || update.isPending}>
          {t.meal.save}
        </Button>
        <Button
          variant="ghost"
          onClick={() => {
            onDone([])
          }}
        >
          {t.meal.cancel}
        </Button>
      </div>
      <div className="flex flex-wrap items-center gap-2 border-t pt-3">
        {confirming ? (
          <>
            <span className="text-sm">{t.meal.confirmDelete}</span>
            <Button
              variant="destructive"
              disabled={remove.isPending}
              onClick={() => {
                remove.mutate({ params: { path: { id: meal.id } } })
              }}
            >
              {t.meal.yesDelete}
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setConfirming(false)
              }}
            >
              {t.meal.keepMeal}
            </Button>
          </>
        ) : (
          <Button
            variant="destructive"
            onClick={() => {
              setConfirming(true)
            }}
          >
            {t.meal.delete}
          </Button>
        )}
        {remove.isError && <SaveStatus change={remove} />}
      </div>
    </form>
  )
}
