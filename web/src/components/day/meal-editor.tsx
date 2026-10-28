import { ArrowLeftRight, BookmarkPlus, RotateCcw, Trash2, X } from 'lucide-react'
import {
  Fragment,
  useEffect,
  useId,
  useRef,
  useState,
  type RefObject,
  type SubmitEvent,
} from 'react'

import { statusOf } from '@/api/errors'
import { useFoods } from '@/api/foods'
import { useDeleteMeal, useUpdateMeal } from '@/api/meals'
import type { components } from '@/api/schema'
import type { Item, Meal } from '@/api/types'
import { EntryRow } from '@/components/day/entry-row'
import { FoodPicker } from '@/components/day/food-picker'
import {
  amountOf,
  converted,
  entryValues,
  estimateEntry,
  foodEntry,
  foodItem,
  itemName,
  itemOf,
  most,
  nutrientsIn,
  quantityIn,
  servingOf,
  shownAmount,
  tooMuch,
  unitIn,
  unitsOf,
  type Amount,
  type Entry,
  type Values,
} from '@/components/day/meal-entries'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { TimeInput } from '@/components/ui/time-input'
import { addDays, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'
import { clockAt, mealInstant, nightAfter, parseClock, toCome } from '@/lib/zoned'

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
// fields' texts as the editor opened with them, its name, and the note it
// had.
const values = ['kcal', 'protein', 'carbs', 'fat'] as const

interface EstimateStart {
  texts: Record<(typeof values)[number], string>
  name: string
  note: string | undefined
}

// fieldKey is an item's amount field's key: the item, of its food.
const fieldKey = (item: Item) => `${item.id} ${String(item.food_id)}`

// fieldOf is the name of the field of an estimate's value, its name or
// an item's unit.
const fieldOf = (item: Item, v: (typeof values)[number] | 'name' | 'unit') => `${item.id}:${v}`

// MealEditor changes a meal's description, day (today or one before),
// time and note, and its items: the amount and unit of each of a food,
// the name and values of each estimate, an estimate saved as a food of
// the user's, another food in the place of any (each keeps its note),
// which stay, and new ones (a food of the user's or an estimate, as a
// meal logged by hand has them); in one change saved at once, or deletes
// the meal. It sends only what was changed since it opened, so a change
// made elsewhere meanwhile stays. A toast says what was done, with what
// the server warned of it, the day a meal moved to, and Undo: no
// question before a delete, as Undo takes it back. It lives in a sheet,
// its total said as it changes, an item a change makes bring other kcal
// marked with what it brought, the save and the delete at its foot.
// onDone closes it.
export function MealEditor({
  meal,
  date,
  today,
  timeZone,
  startHour,
  first,
  onDone,
}: {
  meal: Meal
  date: string
  // The user's logical today: a meal is of it or a day before.
  today: string
  // The user's clock: its time zone and the hour a day starts at.
  timeZone: string
  startHour: number
  // The field the sheet's focus goes to when it opens.
  first?: RefObject<HTMLInputElement | null>
  onDone: () => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const what = meal.description || t.toast.aMeal
  const update = useUpdateMeal(date, (answer) => {
    const moved = answer.day.date !== date
    sayChange(
      {
        title: moved ? t.toast.moved(f.date(answer.day.date, today)) : t.toast.changed(what),
        lines: answer.warnings.map((w) => t.meal.warnings[w.code]),
        to: moved ? answer.day.date : undefined,
      },
      { last: { action: 'meal.replace', entity: meal.id }, date },
    )
  })
  const remove = useDeleteMeal(date, () => {
    sayChange(
      { title: t.toast.deleted(what) },
      { last: { action: 'meal.delete', entity: meal.id }, date },
    )
  })
  // The items' foods, for the units each can be given in.
  const foodIds = [...new Set(meal.items.flatMap((i) => (i.food_id ? [i.food_id] : [])))]
  const foods = useFoods(foodIds)
  const foodOf = (foodId: string | undefined) =>
    foodId === undefined ? undefined : foods[foodIds.indexOf(foodId)]
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
  // The time in its field, to say when it is of the night after the day.
  const [clock, setClock] = useState(openedWith.time)
  const night = nightAfter(clock, startHour)
  // Each estimate's values as the editor opened with them, as the amounts;
  // its name as the app shows it, which left so keeps the one it has.
  const estimateStart = (item: Item): EstimateStart | undefined =>
    item.food_id === undefined
      ? {
          texts: {
            kcal: f.field(item.nutrients.kcal),
            protein: f.field(item.nutrients.protein),
            carbs: f.field(item.nutrients.carbs),
            fat: f.field(item.nutrients.fat),
          },
          name: itemName(item, t),
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
  // A time still to come: a meal is logged once eaten.
  const [ahead, setAhead] = useState(false)
  // What is typed in each amount's field, by the item and its food, for
  // a unit that agrees with it.
  const [typing, setTyping] = useState<Readonly<Record<string, string>>>({})
  // The unit chosen in each amount's field, by the same key: the number
  // typed is worked out from it when another is chosen. And the number a
  // unit chosen worked out, before it was rounded to be shown, until the
  // amount is typed: it is what is sent, and chosen back, the amount is
  // as it was.
  const [chosen, setChosen] = useState<Readonly<Record<string, string>>>({})
  const [exact, setExact] = useState<Readonly<Record<string, number>>>({})
  // The items added, each under an id made when it was, so the change
  // sent again adds it once; those that cannot be sent as they are; and
  // the one just added, whose first field takes the focus. One the meal
  // has (a change that got no answer added it) is the meal's from then
  // on, changed or removed as its others are, and never added again, not
  // even once it is removed elsewhere.
  const [drafts, setDrafts] = useState<Entry[]>([])
  const inMeal = (e: Entry) => meal.items.some((item) => item.id === e.key)
  const entries = drafts.filter((e) => !inMeal(e))
  if (entries.length < drafts.length) {
    setDrafts(entries)
  }
  // The items whose place another food takes, as an item added is typed,
  // by the item; and the item whose other food is being found.
  const [swaps, setSwaps] = useState<Readonly<Record<string, Entry>>>({})
  const [finding, setFinding] = useState<string | undefined>(undefined)
  // The estimates to save as foods of the user's, a portion of each.
  const [asFoods, setAsFoods] = useState<ReadonlySet<string>>(new Set())
  const [wrongEntries, setWrongEntries] = useState<ReadonlySet<string>>(new Set())
  // The items whose amount is 0, or more than an item may hold (tooMuch).
  const [over, setOver] = useState<ReadonlySet<string>>(new Set())
  const [added, setAdded] = useState<string | undefined>(undefined)
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
  // The meal's note, seldom given, has its field only once it is asked
  // for, which then takes the focus.
  const [noting, setNoting] = useState(false)
  const noteField = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    if (noting) {
      noteField.current?.focus()
    }
  }, [noting])
  // What is typed in each estimate's values, by fieldOf, for the total.
  const [valueTyping, setValueTyping] = useState<Readonly<Record<string, string>>>({})
  // A time typed that is no time (parseClock).
  const [badTime, setBadTime] = useState(false)

  // nowOf is what an item brings as the editor has it now, for the total:
  // another food in its place, that food's; an estimate's values as
  // typed; a food's amount typed anew (as submit sends it), worked out
  // from its food as it is now, as the server works it out; else what it
  // brings now. Undefined while a number typed is none, or a food has not
  // come.
  function nowOf(item: Item): Values | undefined {
    const swap = swaps[item.id]
    if (swap !== undefined) {
      return entryValues(swap, f.parse)
    }
    if (item.food_id === undefined) {
      const typed = values.map((v) =>
        f.parse(valueTyping[fieldOf(item, v)] ?? estimateOf(item)?.texts[v] ?? ''),
      )
      const [kcal, protein, carbs, fat] = typed
      return kcal === undefined || protein === undefined || carbs === undefined || fat === undefined
        ? undefined
        : { kcal, protein, carbs, fat }
    }
    const was = startOf(item)
    const field = `${item.id} ${String(was?.foodId)}`
    if (was === undefined || (typing[field] === undefined && chosen[field] === undefined)) {
      return item.nutrients
    }
    const worked = exact[field]
    const n = worked ?? f.parse(typing[field] ?? was.text)
    const food = foodOf(item.food_id)
    const unitWas = food ? unitIn(food, was.amount.key) : was.amount.key
    const unit = chosen[field] ?? unitWas
    const anew =
      unit !== unitWas || (worked === undefined ? n !== f.parse(was.text) : n !== was.amount.n)
    if (n !== undefined && !anew) {
      return item.nutrients
    }
    return n === undefined || food === undefined ? undefined : nutrientsIn(food, unit, n)
  }
  const kept = meal.items.filter((item) => !removed.has(item.id))
  const total = [
    ...kept.map((item) => nowOf(item) ?? item.nutrients),
    ...entries.map((e) => entryValues(e, f.parse) ?? { kcal: 0, protein: 0, carbs: 0, fat: 0 }),
  ].reduce(
    (sum, v) => ({
      kcal: sum.kcal + v.kcal,
      protein: sum.protein + v.protein,
      carbs: sum.carbs + v.carbs,
      fat: sum.fat + v.fat,
    }),
    { kcal: 0, protein: 0, carbs: 0, fat: 0 },
  )
  const change = Math.round(total.kcal) - Math.round(meal.total.kcal)
  // The days a meal is of: today and the 30 before it, and the meal's
  // own, when it is older.
  const days = [
    ...Array.from({ length: 31 }, (_, i) => addDays(today, -i)),
    ...(date < addDays(today, -30) ? [date] : []),
  ]
  const dayName = (d: string) =>
    d === today ? t.day.today : d === addDays(today, -1) ? t.meal.yesterday : f.date(d, today)

  function send(body: Change) {
    update.mutate({ params: { path: { id: meal.id } }, body }, { onSuccess: onDone })
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
    const mealNote = text('note')
    const day = text('day') ?? date
    // A cleared time keeps the one it had; one that is no time is said.
    const typedTime = text('time')
    const time =
      typedTime === undefined || typedTime === '' ? openedWith.time : parseClock(typedTime)
    setBadTime(time === undefined)
    if (time === undefined) {
      return
    }
    const fields: Change = {
      ...(description !== undefined && description !== openedWith.description.trim()
        ? { description }
        : {}),
      ...(day !== date || time !== openedWith.time
        ? { eaten_at: mealInstant(day, time, timeZone, startHour) }
        : {}),
      ...(mealNote !== undefined && mealNote !== openedWith.note.trim() ? { note: mealNote } : {}),
    }
    const ops: Op[] = []
    const notNumbers = new Set<string>()
    const overs = new Set<string>()
    const bad = new Set<string>()
    for (const item of meal.items) {
      if (removed.has(item.id)) {
        ops.push({ op: 'remove', item_id: item.id })
        continue
      }
      const estimate = estimateOf(item)
      const swap = swaps[item.id]
      if (swap !== undefined) {
        // Another food in its place, under its id, with its note.
        const input = itemOf(swap, f.parse)
        if (input === undefined) {
          bad.add(swap.key)
        } else {
          const { id: _new, ...rest } = input
          ops.push({
            op: 'replace',
            item_id: item.id,
            item: item.note === undefined ? rest : { ...rest, note: item.note },
          })
        }
        continue
      }
      if (estimate !== undefined) {
        // Only the values typed anew; the others as they are now. A name
        // cleared keeps the one it had.
        const nutrients = { ...item.nutrients }
        const nameTyped = text(fieldOf(item, 'name'))
        const name =
          nameTyped !== undefined && nameTyped !== '' && nameTyped !== estimate.name.trim()
            ? nameTyped
            : item.name
        const asFood = asFoods.has(item.id)
        let changed = name !== item.name || asFood
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
          const had = item.note === undefined ? {} : { note: item.note }
          // Saved as a food, the plate is a portion of it, as it is typed.
          const input = asFood
            ? {
                food: { name, basis: 'portion' as const, source: 'estimate' as const, nutrients },
                portions: 1,
                ...had,
              }
            : { name, nutrients, ...had }
          ops.push({ op: 'replace', item_id: item.id, item: input })
        }
        continue
      }
      const was = startOf(item)
      const typed = typedIn.get(item.id)
      if (was === undefined || item.food_id === undefined || typeof typed !== 'string') {
        continue
      }
      const food = { ...item, food_id: item.food_id }
      // As a unit chosen worked it out, while the field shows it rounded.
      const worked = exact[`${item.id} ${String(was.foodId)}`]
      const n = worked ?? f.parse(typed)
      // A unit chosen anew: the number is of it.
      const unitTyped = text(fieldOf(item, 'unit'))
      const itsFood = foodOf(item.food_id)
      const unitWas = itsFood ? unitIn(itsFood, was.amount.key) : was.amount.key
      const unit = unitTyped !== undefined && unitTyped !== unitWas ? unitTyped : undefined
      // Only an amount typed anew, or in a unit chosen anew, is sent: one
      // left as it started, rounded to what the field shows, is not sent
      // rounded, nor one a unit chosen and chosen back gave back. An item
      // left as it was keeps what it brought, though past what its food
      // as corrected since would hold.
      const anew =
        n !== undefined &&
        (unit !== undefined ||
          (worked === undefined ? n !== f.parse(was.text) : n !== was.amount.n))
      if (n === undefined) {
        notNumbers.add(item.id)
      } else if (!anew) {
        continue
      } else if (n <= 0 || tooMuch(itsFood, unit ?? unitWas, n)) {
        overs.add(item.id)
      } else if (unit !== undefined) {
        ops.push({
          op: 'replace',
          item_id: item.id,
          item: { ...foodItem(food), ...quantityIn(unit, n) },
        })
      } else {
        ops.push({ op: 'replace', item_id: item.id, item: was.amount.with(food, n) })
      }
    }
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
    setOver(overs)
    const after = fields.eaten_at !== undefined && toCome(fields.eaten_at)
    setAhead(after)
    if (notNumbers.size > 0 || bad.size > 0 || overs.size > 0 || after) {
      return
    }
    if (ops.length === 0 && Object.keys(fields).length === 0) {
      onDone()
      return
    }
    send({ ...fields, ...(ops.length > 0 ? { ops } : {}) })
  }

  return (
    <form ref={form} onSubmit={submit} noValidate className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
        <label className="block space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.log.description}</span>
          <input
            ref={first}
            name="description"
            defaultValue={openedWith.description}
            maxLength={200}
            autoComplete="off"
            className={inputClass}
          />
        </label>
        <div className="flex gap-3">
          <label className="min-w-0 flex-1 space-y-1">
            <span className={cn(fieldLabel, 'block')}>{t.meal.day}</span>
            <select name="day" defaultValue={date} className={inputClass}>
              {days.map((d) => (
                <option key={d} value={d}>
                  {dayName(d)}
                </option>
              ))}
            </select>
          </label>
          <label className="w-24 space-y-1">
            <span className={cn(fieldLabel, 'block')}>{t.log.time}</span>
            <TimeInput
              name="time"
              defaultValue={openedWith.time}
              aria-invalid={ahead || badTime}
              aria-describedby={
                [ahead || badTime ? `${id}-time` : '', night ? `${id}-night` : '']
                  .join(' ')
                  .trim() || undefined
              }
              onChange={(e) => {
                // A cleared field keeps the time it had.
                const typed = parseClock(e.currentTarget.value)
                if (typed !== undefined) {
                  setClock(typed)
                }
                setBadTime(false)
                setAhead(false)
              }}
              onBlur={(e) => {
                // As the time it reads, 19:30 for 1930.
                const typed = e.currentTarget.value
                e.currentTarget.value = parseClock(typed) ?? typed
              }}
            />
          </label>
        </div>
        {night && (
          <p id={`${id}-night`} className="text-xs text-muted-foreground">
            {t.log.nightAfter(clock)}
          </p>
        )}
        {/* The meal as it would be saved, said as it changes. */}
        <div
          aria-live="polite"
          className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg bg-surface-2 px-4 py-3"
        >
          <p>
            <span className="text-h2">{t.unit.kcal(f.number(total.kcal))}</span>
            {change !== 0 && (
              <span className="block text-caption font-semibold text-primary">
                {t.meal.fromChanges(f.signed(change))}
              </span>
            )}
          </p>
          <p className="ml-auto text-small text-muted-foreground">
            {t.day.short.protein} {f.grams(total.protein)} · {t.day.short.carbs}{' '}
            {f.grams(total.carbs)} · {t.day.short.fat} {f.grams(total.fat)}
          </p>
        </div>
        <h3 className="text-h3">{t.meal.itemsTitle}</h3>
        <ul className="divide-y text-sm">
          {meal.items.map((item) => {
            const name = itemName(item, t)
            const was = startOf(item)
            const now = start(item)
            const gone = removed.has(item.id)
            // What it brings now, and whether a change here makes it bring
            // another energy than it does.
            const brings = nowOf(item)
            const changed =
              !gone &&
              brings !== undefined &&
              Math.round(brings.kcal) !== Math.round(item.nutrients.kcal)
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
                  estimateNow.name !== openedEstimate.name ||
                  values.some((v) => estimateNow.texts[v] !== openedEstimate.texts[v])))
            const field = `${item.id} ${String(was?.foodId)}`
            const typedN = f.parse(typing[field] ?? was?.text ?? '') ?? Number.NaN
            const unit = was?.amount.portions ? t.meal.portions(typedN) : was?.amount.unit
            // The units its food is given in, the one it has among them, once
            // the food has come: a choice only of two or more.
            const itsFood = foodOf(was?.foodId)
            // As the food names it: a serving given as Piece is its piece.
            const unitWas = was && itsFood ? unitIn(itsFood, was.amount.key) : was?.amount.key
            // The one chosen stays among them though its food no longer has
            // it: the amount is of it, which the server refuses rather than
            // the amount being taken in another unit.
            const picked = chosen[field]
            const units =
              unitWas !== undefined && itsFood
                ? [...new Set([unitWas, ...(picked ? [picked] : []), ...unitsOf(itsFood)])]
                : undefined
            const choices = units && units.length > 1 ? units : undefined
            const swap = gone ? undefined : swaps[item.id]
            return (
              <Fragment key={item.id}>
                <li
                  className={cn(
                    'flex flex-wrap items-center gap-2 py-2',
                    changed && '-mx-2 rounded-md bg-primary-soft/60 px-2',
                  )}
                  hidden={swap !== undefined}
                >
                  <span
                    className={cn('min-w-0 flex-1', gone && 'text-muted-foreground line-through')}
                  >
                    {name}
                    {gone && <span className="sr-only"> ({t.meal.removed})</span>}
                    {!gone && brings !== undefined && (
                      <span
                        className={cn(
                          'block text-caption',
                          changed ? 'font-medium text-primary' : 'text-muted-foreground',
                        )}
                      >
                        {t.unit.kcal(f.number(brings.kcal))} · {t.day.short.protein}{' '}
                        {f.grams(brings.protein)}
                        {changed && ` · ${t.meal.was(f.number(item.nutrients.kcal))}`}
                      </span>
                    )}
                    {moved && !gone && (
                      <span className="block text-xs text-warning">{t.meal.changedElsewhere}</span>
                    )}
                  </span>
                  {was && (
                    // Hidden, not gone, while the item is to be removed: kept
                    // again, it has what was typed in it.
                    <span className="flex items-center gap-1" hidden={gone}>
                      <span className={cn('relative', choices ? 'w-20' : 'w-32')}>
                        <input
                          // Of another food now, the field starts again.
                          key={was.foodId}
                          name={item.id}
                          aria-label={t.meal.amount(name)}
                          inputMode="decimal"
                          autoComplete="off"
                          defaultValue={was.text}
                          onChange={(e) => {
                            const next = e.currentTarget.value
                            setTyping((ts) => ({ ...ts, [field]: next }))
                            // Typed: what a unit chosen worked out is no more.
                            setExact(({ [field]: _typed, ...es }) => es)
                          }}
                          aria-invalid={wrong.has(item.id) || over.has(item.id)}
                          aria-describedby={
                            wrong.has(item.id)
                              ? `${id}-wrong`
                              : over.has(item.id)
                                ? `${id}-wrong-entry`
                                : undefined
                          }
                          className={cn(inputClass, 'tabular-nums', !choices && unit && 'pr-14')}
                        />
                        {!choices && unit && (
                          <span className="pointer-events-none absolute inset-y-0 right-2 flex max-w-12 items-center truncate text-xs text-muted-foreground">
                            {unit}
                          </span>
                        )}
                      </span>
                      {choices && itsFood && (
                        <select
                          // Of another food now, the choice starts again.
                          key={was.foodId}
                          name={fieldOf(item, 'unit')}
                          aria-label={t.log.unit(name)}
                          // Held by the editor, not the page: a unit renamed or
                          // gone elsewhere never changes it unseen.
                          value={picked ?? unitWas}
                          className={cn(inputClass, 'w-auto max-w-28')}
                          onChange={(e) => {
                            // The amount is worked out anew in the unit chosen,
                            // when the food says the grams of both.
                            const unitTo = e.currentTarget.value
                            const amount = e.currentTarget.form?.elements.namedItem(item.id)
                            setChosen((cs) => ({ ...cs, [field]: unitTo }))
                            if (!(amount instanceof HTMLInputElement)) {
                              return
                            }
                            // From what a unit chosen worked out, or the
                            // amount as it was while it is not typed, or as it
                            // is typed.
                            const base =
                              exact[field] ?? (typing[field] === undefined ? was.amount.n : typedN)
                            const worked = Number.isFinite(base)
                              ? converted(
                                  itsFood,
                                  chosen[field] ?? unitWas ?? was.amount.key,
                                  unitTo,
                                  base,
                                )
                              : undefined
                            if (worked === undefined) {
                              // Not to be worked out: the amount is typed anew
                              // in the unit chosen, not taken over in it.
                              amount.value = ''
                              setTyping((ts) => ({ ...ts, [field]: '' }))
                              setExact(({ [field]: _gone, ...es }) => es)
                            } else {
                              // Back in its own unit, the amount it was, not
                              // one a hair off it.
                              const n =
                                unitTo === unitWas &&
                                Math.abs(worked - was.amount.n) <= 1e-9 * Math.max(1, was.amount.n)
                                  ? was.amount.n
                                  : worked
                              const next = shownAmount(n, f.field)
                              amount.value = next
                              setTyping((ts) => ({ ...ts, [field]: next }))
                              setExact((es) => ({ ...es, [field]: n }))
                            }
                          }}
                        >
                          {choices.map((u) => (
                            <option key={u} value={u}>
                              {u === 'portion' ? t.meal.portions(typedN) : (servingOf(u) ?? u)}
                            </option>
                          ))}
                        </select>
                      )}
                    </span>
                  )}
                  {estimate && !gone && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-primary aria-pressed:bg-primary-soft"
                      aria-label={t.meal.saveAsFood(name)}
                      aria-pressed={asFoods.has(item.id)}
                      onClick={() => {
                        const next = new Set(asFoods)
                        if (next.has(item.id)) {
                          next.delete(item.id)
                        } else {
                          next.add(item.id)
                        }
                        setAsFoods(next)
                      }}
                    >
                      <BookmarkPlus aria-hidden />
                      {t.meal.asFoodShort}
                    </Button>
                  )}
                  {!gone && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t.meal.anotherFood(name)}
                      aria-expanded={finding === item.id}
                      onClick={() => {
                        setFinding(finding === item.id ? undefined : item.id)
                      }}
                    >
                      <ArrowLeftRight aria-hidden />
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={gone ? t.meal.keep(name) : t.meal.remove(name)}
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
                  {estimate && asFoods.has(item.id) && !gone && (
                    <span className="basis-full text-xs text-muted-foreground">
                      {t.meal.asFood}
                    </span>
                  )}
                  {estimate && (
                    <input
                      name={fieldOf(item, 'name')}
                      aria-label={t.meal.value(t.log.estimateName, name)}
                      defaultValue={estimate.name}
                      maxLength={200}
                      autoComplete="off"
                      hidden={gone}
                      className={cn(inputClass, 'basis-full')}
                    />
                  )}
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
                            aria-label={t.meal.value(t.foods.nutrient[v], name)}
                            inputMode="decimal"
                            autoComplete="off"
                            defaultValue={estimate.texts[v]}
                            onChange={(e) => {
                              const next = e.currentTarget.value
                              setValueTyping((ts) => ({ ...ts, [fieldOf(item, v)]: next }))
                            }}
                            aria-invalid={wrong.has(fieldOf(item, v))}
                            aria-describedby={
                              wrong.has(fieldOf(item, v)) ? `${id}-wrong` : undefined
                            }
                            className={cn(inputClass, 'px-2.5 tabular-nums')}
                          />
                        </label>
                      ))}
                    </span>
                  )}
                  {finding === item.id && !gone && (
                    <span className="basis-full">
                      <FoodPicker
                        label={t.meal.findInPlaceOf(name)}
                        onPick={(food) => {
                          const entry = foodEntry(food, f.field)
                          setSwaps((ss) => ({ ...ss, [item.id]: entry }))
                          setFinding(undefined)
                          setAdded(entry.key)
                        }}
                      />
                    </span>
                  )}
                </li>
                {swap && (
                  <>
                    <li className="pt-2 text-xs text-muted-foreground">{t.meal.inPlaceOf(name)}</li>
                    <EntryRow
                      entry={swap}
                      wrong={wrongEntries.has(swap.key)}
                      describedBy={wrongEntries.has(swap.key) ? `${id}-wrong-entry` : undefined}
                      focus={swap.key === added}
                      onChange={(other) => {
                        setSwaps((ss) => ({ ...ss, [item.id]: other }))
                      }}
                      onRemove={() => {
                        const { [item.id]: _gone, ...rest } = swaps
                        setSwaps(rest)
                      }}
                    />
                  </>
                )}
              </Fragment>
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
            className="border-dashed"
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
        {noting ? (
          <label className="block space-y-1">
            <span className={cn(fieldLabel, 'block')}>{t.meal.note}</span>
            <textarea
              ref={noteField}
              name="note"
              defaultValue={openedWith.note}
              maxLength={2000}
              rows={2}
              className={cn(inputClass, 'h-auto py-2')}
            />
          </label>
        ) : (
          <Button
            variant="link"
            className="h-auto p-0 text-small"
            onClick={() => {
              setNoting(true)
            }}
          >
            {openedWith.note === '' ? t.meal.addNote : t.meal.changeNote}
          </Button>
        )}
      </div>
      {/* The foot stays, whatever scrolls: the delete apart from the save. */}
      <div className="space-y-2 border-t p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
        {wrong.size > 0 && (
          <p id={`${id}-wrong`} role="alert" className="text-xs text-destructive">
            {t.day.notANumber}
          </p>
        )}
        {(over.size > 0 ||
          [...entries, ...Object.values(swaps)].some((e) => wrongEntries.has(e.key))) && (
          <p id={`${id}-wrong-entry`} role="alert" className="text-xs text-destructive">
            {t.log.wrong(most(f.number))}
          </p>
        )}
        {(ahead || badTime) && (
          <p id={`${id}-time`} role="alert" className="text-xs text-destructive">
            {badTime ? t.meal.timeFormat : t.meal.timeToCome}
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
        {/* A meal no longer there (404) was deleted: by this delete, its
            answer lost, or elsewhere. */}
        {remove.isError && statusOf(remove.error) !== 404 && <SaveStatus change={remove} />}
        <div className="flex gap-2">
          <Button
            variant="destructive"
            size="icon"
            className="h-13 w-13"
            aria-label={t.meal.delete}
            disabled={remove.isPending}
            onClick={() => {
              remove.mutate({ params: { path: { id: meal.id } } })
            }}
          >
            <Trash2 aria-hidden />
          </Button>
          <Button
            type="submit"
            className="h-13 flex-1 text-base"
            disabled={none || update.isPending}
          >
            {t.meal.save}
          </Button>
        </div>
      </div>
    </form>
  )
}
