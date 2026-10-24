import { RotateCcw, X } from 'lucide-react'
import { useEffect, useId, useRef, useState, type SubmitEvent } from 'react'

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
type Change = components['schemas']['UpdateMealBody']

// An amount is how much of an item was given, as it was given (grams,
// portions, servings), and the item that puts another number in its
// place, given the same way: the item's food (as it is when it is sent)
// worked out again from the food as it is now, so a food corrected since
// gives its new values. Only an item of a food has one: an estimate is of
// a whole plate. Its unit is a serving's name or the food's unit, or, in
// portions, the word for as many as are typed.
interface Amount {
  n: number
  unit: string | undefined
  portions?: true
  with: (item: Item & { food_id: string }, n: number) => ItemInput
}

function base(item: Item & { food_id: string }): ItemInput {
  return item.note === undefined
    ? { food_id: item.food_id }
    : { food_id: item.food_id, note: item.note }
}

function amountOf(item: Item): Amount | undefined {
  if (item.food_id === undefined) {
    return undefined
  }
  const q = item.quantity
  if (q.servings !== undefined && q.serving !== undefined) {
    const serving = q.serving
    return {
      n: q.servings,
      unit: serving,
      with: (it, n) => ({ ...base(it), serving, servings: n }),
    }
  }
  if (q.portions !== undefined) {
    return {
      n: q.portions,
      unit: undefined,
      portions: true,
      with: (it, n) => ({ ...base(it), portions: n }),
    }
  }
  if (q.amount !== undefined) {
    // Given without a unit, it is the food's, and stays so.
    const given = q.unit
    return {
      n: q.amount,
      unit: given ?? (item.unit === 'portion' ? undefined : item.unit),
      with: (it, n) =>
        given === undefined ? { ...base(it), amount: n } : { ...base(it), amount: n, unit: given },
    }
  }
  return undefined
}

// A start is an item's amount as the editor opened with it, of its food
// then, and the note it had.
interface Start {
  text: string
  amount: Amount
  foodId: string | undefined
  note: string | undefined
}

// MealEditor changes a meal's items, the amount of each of a food and
// which stay, in one change saved at once, or deletes the meal after
// asking. It sends only what was changed since it opened, so a change
// made elsewhere meanwhile stays. onDone gives the server's warnings of
// the change.
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
  const id = useId()
  const update = useUpdateMeal(date)
  const remove = useDeleteMeal(date)
  // Each amount as the editor opened with it: what its field starts with,
  // and what a number typed in it means, though the item changes
  // elsewhere meanwhile. An item added since, or of another food now,
  // starts as it is.
  const start = (item: Item): Start | undefined => {
    const amount = amountOf(item)
    return amount && { text: f.field(amount.n), amount, foodId: item.food_id, note: item.note }
  }
  const [initial] = useState(
    () =>
      new Map(
        meal.items.flatMap((item) => {
          const s = start(item)
          return s ? [[item.id, s] as const] : []
        }),
      ),
  )
  const startOf = (item: Item) => {
    const opened = initial.get(item.id)
    return opened?.foodId === item.food_id ? opened : start(item)
  }
  // The items to remove, and those whose amount is not a number.
  const [removed, setRemoved] = useState<ReadonlySet<string>>(new Set())
  const [wrong, setWrong] = useState<ReadonlySet<string>>(new Set())
  // What is typed in each amount's field, by the item and its food, for
  // a unit that agrees with it.
  const [typing, setTyping] = useState<Readonly<Record<string, string>>>({})
  const [confirming, setConfirming] = useState(false)
  // Of the items there now: one removed elsewhere is not counted.
  const none = meal.items.every((item) => removed.has(item.id))
  // A change the server refuses (422, or 409 for an item changed since)
  // would be refused again.
  // A meal no longer there (404) was deleted elsewhere: the days are asked
  // again, and its card goes.
  const refused = update.isError && [404, 409, 422].includes(statusOf(update.error) ?? 0)
  const form = useRef<HTMLFormElement>(null)
  const keep = useRef<HTMLButtonElement>(null)

  // The editor takes the focus from the pencil that opened it.
  useEffect(() => {
    form.current?.querySelector<HTMLElement>('input, button')?.focus()
  }, [])
  // Asked whether to delete, the focus is on keeping the meal.
  useEffect(() => {
    if (confirming) {
      keep.current?.focus()
    }
  }, [confirming])

  function send(body: Change) {
    update.mutate(
      { params: { path: { id: meal.id } }, body },
      {
        onSuccess: (answer) => {
          onDone(answer.warnings)
        },
      },
    )
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    // A meal keeps an item: the save is off, and so is sending it again.
    if (none) {
      return
    }
    const typedIn = new FormData(event.currentTarget)
    const ops: Op[] = []
    const notNumbers = new Set<string>()
    for (const item of meal.items) {
      if (removed.has(item.id)) {
        ops.push({ op: 'remove', item_id: item.id })
        continue
      }
      const was = startOf(item)
      const typed = typedIn.get(item.id)
      if (was === undefined || item.food_id === undefined || typeof typed !== 'string') {
        continue
      }
      const food = { ...item, food_id: item.food_id }
      const n = f.parse(typed)
      if (n === undefined) {
        notNumbers.add(item.id)
      } else if (n !== f.parse(was.text)) {
        // Only an amount typed anew: one left as it started, rounded to
        // what the field shows, is not sent rounded.
        ops.push({ op: 'replace', item_id: item.id, item: was.amount.with(food, n) })
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
    send({ ops })
  }

  return (
    <form ref={form} onSubmit={submit} noValidate className="space-y-3">
      <ul className="divide-y text-sm">
        {meal.items.map((item) => {
          const was = startOf(item)
          const now = start(item)
          const gone = removed.has(item.id)
          // Changed elsewhere since the editor opened: its field keeps what
          // it opened with.
          const opened = initial.get(item.id)
          const moved =
            opened !== undefined &&
            (now?.text !== opened.text ||
              now.amount.unit !== opened.amount.unit ||
              now.amount.portions !== opened.amount.portions ||
              now.foodId !== opened.foodId ||
              now.note !== opened.note)
          const field = `${item.id} ${String(was?.foodId)}`
          const unit = was?.amount.portions
            ? t.meal.portions(f.parse(typing[field] ?? was.text) ?? Number.NaN)
            : was?.amount.unit
          return (
            <li key={item.id} className="flex flex-wrap items-center gap-2 py-2">
              <span className={cn('min-w-0 flex-1', gone && 'text-muted-foreground line-through')}>
                {item.name}
                {gone && <span className="sr-only"> ({t.meal.removed})</span>}
                {moved && !gone && (
                  <span className="block text-xs text-amber-700 dark:text-amber-400">
                    {t.meal.changedElsewhere}
                  </span>
                )}
              </span>
              {was && (
                // Hidden, not gone, while the item is to be removed: kept
                // again, it has what was typed in it.
                <span className="relative w-32" hidden={gone}>
                  <input
                    // Of another food now, the field starts again.
                    key={was.foodId}
                    name={item.id}
                    aria-label={t.meal.amount(item.name)}
                    inputMode="decimal"
                    autoComplete="off"
                    defaultValue={was.text}
                    onChange={(e) => {
                      const next = e.currentTarget.value
                      setTyping((ts) => ({ ...ts, [field]: next }))
                    }}
                    aria-invalid={wrong.has(item.id)}
                    aria-describedby={wrong.has(item.id) ? `${id}-wrong` : undefined}
                    className={cn(inputClass, 'h-9 tabular-nums', unit && 'pr-14')}
                  />
                  {unit && (
                    <span className="pointer-events-none absolute inset-y-0 right-2 flex max-w-12 items-center truncate text-xs text-muted-foreground">
                      {unit}
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
      <p className="text-xs text-muted-foreground">{t.meal.recomputed}</p>
      {wrong.size > 0 && (
        <p id={`${id}-wrong`} role="alert" className="text-xs text-destructive">
          {t.day.notANumber}
        </p>
      )}
      {none && <p className="text-xs text-muted-foreground">{t.meal.oneItem}</p>}
      {refused ? (
        <p role="alert" className="text-xs text-destructive">
          {t.meal.refused}
        </p>
      ) : (
        <SaveStatus
          change={update}
          // Again, with the meal as it is typed now: an op done already
          // the server takes for done.
          onRetry={() => {
            form.current?.requestSubmit()
          }}
        />
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
              ref={keep}
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
        {/* A meal no longer there (404) was deleted: by this delete, its
            answer lost, or elsewhere. */}
        {remove.isError && statusOf(remove.error) !== 404 && <SaveStatus change={remove} />}
      </div>
    </form>
  )
}
