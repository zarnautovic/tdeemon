import { RotateCcw, X } from 'lucide-react'
import { useEffect, useId, useRef, useState, type SubmitEvent } from 'react'

import { statusOf } from '@/api/errors'
import { useDeleteMeal, useUpdateMeal } from '@/api/meals'
import type { components } from '@/api/schema'
import type { Item, Meal, Warning } from '@/api/types'
import { EntryRow } from '@/components/day/entry-row'
import { FoodPicker } from '@/components/day/food-picker'
import {
  amountOf,
  estimateEntry,
  foodEntry,
  itemOf,
  type Amount,
  type Entry,
} from '@/components/day/meal-entries'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'
import { clockAt, mealInstant } from '@/lib/zoned'

type Op = components['schemas']['MealOp']
type Change = components['schemas']['UpdateMealBody']

// A start is an item's amount as the editor opened with it, of its food
// then, and the note it had.
interface Start {
  text: string
  amount: Amount
  foodId: string | undefined
  note: string | undefined
}

// The values of an estimate the editor changes, and a start of them: the
// fields' texts as the editor opened with them, and the note it had.
const values = ['kcal', 'protein', 'carbs', 'fat'] as const

interface EstimateStart {
  texts: Record<(typeof values)[number], string>
  note: string | undefined
}

// fieldKey is an item's amount field's key: the item, of its food.
const fieldKey = (item: Item) => `${item.id} ${String(item.food_id)}`

// fieldOf is the name of the field of an estimate's value.
const fieldOf = (item: Item, v: (typeof values)[number]) => `${item.id}:${v}`

// MealEditor changes a meal's description, time (within its day) and
// note, and its items: the amount of each of a food, the values of each
// estimate, which stay, and new ones (a food of the user's or an
// estimate, as a meal logged by hand has them); in one change saved at
// once, or deletes the meal after
// asking. It sends only what was changed since it opened, so a change
// made elsewhere meanwhile stays. onDone gives the server's warnings of
// the change.
export function MealEditor({
  meal,
  date,
  timeZone,
  startHour,
  onDone,
}: {
  meal: Meal
  date: string
  // The user's clock: its time zone and the hour a day starts at.
  timeZone: string
  startHour: number
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
  // starts as it is when its field comes, and holds that as the field
  // does (later).
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
    return opened?.foodId === item.food_id ? opened : (later.get(fieldKey(item)) ?? start(item))
  }
  // The meal's words and time as the editor opened with them: only those
  // typed anew are sent.
  const [openedWith] = useState(() => ({
    description: meal.description,
    time: clockAt(Date.parse(meal.eaten_at), timeZone),
    note: meal.note ?? '',
  }))
  // Each estimate's values as the editor opened with them, as the amounts.
  const estimateStart = (item: Item): EstimateStart | undefined =>
    item.food_id === undefined
      ? {
          texts: {
            kcal: f.field(item.nutrients.kcal),
            protein: f.field(item.nutrients.protein),
            carbs: f.field(item.nutrients.carbs),
            fat: f.field(item.nutrients.fat),
          },
          note: item.note,
        }
      : undefined
  const [initialEstimates] = useState(
    () =>
      new Map(
        meal.items.flatMap((item) => {
          const s = estimateStart(item)
          return s ? [[item.id, s] as const] : []
        }),
      ),
  )
  const estimateOf = (item: Item) =>
    item.food_id === undefined
      ? (initialEstimates.get(item.id) ?? laterEstimates.get(item.id) ?? estimateStart(item))
      : undefined
  // The starts of the items that came after the editor opened, held from
  // when they came: a correction made elsewhere since is not undone by a
  // field left as it started.
  const [later, setLater] = useState<ReadonlyMap<string, Start>>(new Map())
  const [laterEstimates, setLaterEstimates] = useState<ReadonlyMap<string, EstimateStart>>(
    new Map(),
  )
  const comes = meal.items.flatMap((item) => {
    const s = initial.get(item.id)?.foodId === item.food_id ? undefined : start(item)
    return s && !later.has(fieldKey(item)) ? [[fieldKey(item), s] as const] : []
  })
  if (comes.length > 0) {
    setLater((m) => new Map([...m, ...comes]))
  }
  const comesEstimates = meal.items.flatMap((item) => {
    const s =
      initialEstimates.has(item.id) || laterEstimates.has(item.id) ? undefined : estimateStart(item)
    return s ? [[item.id, s] as const] : []
  })
  if (comesEstimates.length > 0) {
    setLaterEstimates((m) => new Map([...m, ...comesEstimates]))
  }
  // The items to remove, and the fields that are not a number (an
  // amount's by its item, an estimate's value's by fieldOf).
  const [removed, setRemoved] = useState<ReadonlySet<string>>(new Set())
  const [wrong, setWrong] = useState<ReadonlySet<string>>(new Set())
  // What is typed in each amount's field, by the item and its food, for
  // a unit that agrees with it.
  const [typing, setTyping] = useState<Readonly<Record<string, string>>>({})
  // The items added, each under an id made when it was, so the change
  // sent again adds it once; those that cannot be sent as they are; and
  // the one just added, whose first field takes the focus. One the meal
  // has already (a change that got no answer added it) is the meal's,
  // changed or removed as its others are, and not added again.
  const [drafts, setDrafts] = useState<Entry[]>([])
  const entries = drafts.filter((e) => !meal.items.some((item) => item.id === e.key))
  const [wrongEntries, setWrongEntries] = useState<ReadonlySet<string>>(new Set())
  const [added, setAdded] = useState<string | undefined>(undefined)
  const [confirming, setConfirming] = useState(false)
  // Of the items there now: one removed elsewhere is not counted.
  const none = entries.length === 0 && meal.items.every((item) => removed.has(item.id))
  // A change the server refuses (422, or 409 for an item changed since)
  // would be refused again.
  // A meal no longer there (404) was deleted elsewhere: the days are asked
  // again, and its card goes.
  const refused = update.isError && [404, 409, 422].includes(statusOf(update.error) ?? 0)
  // Sent, and no answer said whether it was taken: the items added hold
  // still until it is sent again, as an item added under its id already
  // would refuse other content, and one removed here would stay.
  const held = update.isPending || (update.isError && !refused)
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
    // The server keeps no space at the ends of a text (text.CheckLines).
    const text = (name: string) => {
      const typed = typedIn.get(name)
      return typeof typed === 'string' ? typed.trim() : undefined
    }
    const description = text('description')
    const time = text('time')
    const mealNote = text('note')
    const fields: Change = {
      ...(description !== undefined && description !== openedWith.description.trim()
        ? { description }
        : {}),
      // A cleared time keeps the one it had.
      ...(time !== undefined && /^\d{2}:\d{2}$/.test(time) && time !== openedWith.time
        ? { eaten_at: mealInstant(date, time, timeZone, startHour) }
        : {}),
      ...(mealNote !== undefined && mealNote !== openedWith.note.trim() ? { note: mealNote } : {}),
    }
    const ops: Op[] = []
    const notNumbers = new Set<string>()
    for (const item of meal.items) {
      if (removed.has(item.id)) {
        ops.push({ op: 'remove', item_id: item.id })
        continue
      }
      const estimate = estimateOf(item)
      if (estimate !== undefined) {
        // Only the values typed anew; the others as they are now.
        const nutrients = { ...item.nutrients }
        let changed = false
        for (const v of values) {
          const typed = typedIn.get(fieldOf(item, v))
          const n = typeof typed === 'string' ? f.parse(typed) : undefined
          if (n === undefined) {
            notNumbers.add(fieldOf(item, v))
          } else if (n !== f.parse(estimate.texts[v])) {
            nutrients[v] = n
            changed = true
          }
        }
        if (changed) {
          const note = item.note === undefined ? {} : { note: item.note }
          ops.push({
            op: 'replace',
            item_id: item.id,
            item: { name: item.name, nutrients, ...note },
          })
        }
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
    const bad = new Set<string>()
    for (const entry of entries) {
      const item = itemOf(entry, f.parse)
      if (item === undefined) {
        bad.add(entry.key)
      } else {
        ops.push({ op: 'add', item })
      }
    }
    setWrong(notNumbers)
    setWrongEntries(bad)
    if (notNumbers.size > 0 || bad.size > 0) {
      return
    }
    if (ops.length === 0 && Object.keys(fields).length === 0) {
      onDone([])
      return
    }
    send({ ...fields, ...(ops.length > 0 ? { ops } : {}) })
  }

  return (
    <form ref={form} onSubmit={submit} noValidate className="space-y-3">
      <div className="flex gap-3">
        <label className="flex-1 space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.log.description}</span>
          <input
            name="description"
            defaultValue={openedWith.description}
            maxLength={200}
            autoComplete="off"
            className={inputClass}
          />
        </label>
        <label className="w-28 space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.log.time}</span>
          <input
            name="time"
            type="time"
            defaultValue={openedWith.time}
            className={cn(inputClass, 'tabular-nums')}
          />
        </label>
      </div>
      <label className="block space-y-1">
        <span className={cn(fieldLabel, 'block')}>{t.meal.note}</span>
        <textarea
          name="note"
          defaultValue={openedWith.note}
          maxLength={2000}
          rows={2}
          className={cn(inputClass, 'h-auto py-2')}
        />
      </label>
      <ul className="divide-y text-sm">
        {meal.items.map((item) => {
          const was = startOf(item)
          const now = start(item)
          const gone = removed.has(item.id)
          // Changed elsewhere since the editor opened, or since the item
          // came: its field keeps what it started with.
          const opened = initial.get(item.id) ?? later.get(fieldKey(item))
          const estimate = estimateOf(item)
          const estimateNow = estimateStart(item)
          const openedEstimate = initialEstimates.get(item.id) ?? laterEstimates.get(item.id)
          const moved =
            (opened !== undefined &&
              (now?.text !== opened.text ||
                now.amount.unit !== opened.amount.unit ||
                now.amount.portions !== opened.amount.portions ||
                now.foodId !== opened.foodId ||
                now.note !== opened.note)) ||
            (openedEstimate !== undefined &&
              (estimateNow === undefined ||
                estimateNow.note !== openedEstimate.note ||
                values.some((v) => estimateNow.texts[v] !== openedEstimate.texts[v])))
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
              {estimate && (
                // Hidden, not gone, as an amount's field.
                <span className="grid basis-full grid-cols-4 gap-2" hidden={gone}>
                  {values.map((v) => (
                    <label key={v} className="space-y-1">
                      <span className="block text-xs text-muted-foreground">
                        {t.meal.estimateValue[v]}
                      </span>
                      <input
                        name={fieldOf(item, v)}
                        aria-label={t.meal.value(t.foods.nutrient[v], item.name)}
                        inputMode="decimal"
                        autoComplete="off"
                        defaultValue={estimate.texts[v]}
                        aria-invalid={wrong.has(fieldOf(item, v))}
                        aria-describedby={wrong.has(fieldOf(item, v)) ? `${id}-wrong` : undefined}
                        className={cn(inputClass, 'h-9 tabular-nums')}
                      />
                    </label>
                  ))}
                </span>
              )}
            </li>
          )
        })}
      </ul>
      <fieldset aria-label={t.meal.add} disabled={held} className="min-w-0 space-y-2">
        {entries.length > 0 && (
          <ul className="divide-y">
            {entries.map((entry) => (
              <EntryRow
                key={entry.key}
                entry={entry}
                wrong={wrongEntries.has(entry.key)}
                describedBy={wrongEntries.has(entry.key) ? `${id}-wrong-entry` : undefined}
                focus={entry.key === added}
                onChange={(changed) => {
                  setDrafts((es) => es.map((e) => (e.key === entry.key ? changed : e)))
                }}
                onRemove={() => {
                  setDrafts((es) => es.filter((e) => e.key !== entry.key))
                }}
              />
            ))}
          </ul>
        )}
        <FoodPicker
          onPick={(food) => {
            const entry = foodEntry(food, f.field)
            setDrafts((es) => [...es, entry])
            setAdded(entry.key)
          }}
        />
        <Button
          variant="outline"
          onClick={() => {
            const entry = estimateEntry()
            setDrafts((es) => [...es, entry])
            setAdded(entry.key)
          }}
        >
          {t.log.estimate}
        </Button>
      </fieldset>
      <p className="text-xs text-muted-foreground">{t.meal.recomputed}</p>
      {wrong.size > 0 && (
        <p id={`${id}-wrong`} role="alert" className="text-xs text-destructive">
          {t.day.notANumber}
        </p>
      )}
      {entries.some((e) => wrongEntries.has(e.key)) && (
        <p id={`${id}-wrong-entry`} role="alert" className="text-xs text-destructive">
          {t.log.wrong}
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
