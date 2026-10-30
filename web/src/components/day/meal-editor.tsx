import { Menu } from '@base-ui/react/menu'
import { Select } from '@base-ui/react/select'
import {
  ArrowLeftRight,
  BookmarkPlus,
  CalendarDays,
  Check,
  ChevronDown,
  Ellipsis,
  RotateCcw,
  Trash2,
  X,
} from 'lucide-react'
import {
  Fragment,
  useEffect,
  useId,
  useRef,
  useState,
  type Ref,
  type RefObject,
  type SubmitEvent,
} from 'react'
import { flushSync } from 'react-dom'

import { statusOf } from '@/api/errors'
import { useFoods } from '@/api/foods'
import { useDeleteMeal, useUpdateMeal } from '@/api/meals'
import type { components } from '@/api/schema'
import type { Item, Meal } from '@/api/types'
import { AmountField } from '@/components/day/amount-field'
import { EntryRow } from '@/components/day/entry-row'
import { FoodPicker } from '@/components/day/food-picker'
import {
  amountOf,
  amountWrong,
  converted,
  entryItem,
  entryValues,
  estimateEntry,
  foodEntry,
  foodItem,
  isShare,
  itemName,
  nutrientsIn,
  parseAmount,
  quantityIn,
  sayWrong,
  shownAmount,
  unitIn,
  unitsOf,
  type Amount,
  type Entry,
  type Values,
  type Wrong,
} from '@/components/day/meal-entries'
import { SaveStatus } from '@/components/day/save-status'
import { entryUnitLabel } from '@/components/recipes/recipe-values'
import { recipeUnit, shareField } from '@/components/recipes/share'
import { Button, buttonVariants } from '@/components/ui/button'
import { TimeInput } from '@/components/ui/time-input'
import { addDays, rounded, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass, linkTarget } from '@/lib/styles'
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

// zero is the values of nothing; plus and minus are a's values with b's
// added, or taken away, each.
const zero: Values = { kcal: 0, protein: 0, carbs: 0, fat: 0 }
const plus = (a: Values, b: Values): Values => ({
  kcal: a.kcal + b.kcal,
  protein: a.protein + b.protein,
  carbs: a.carbs + b.carbs,
  fat: a.fat + b.fat,
})
const minus = (a: Values, b: Values): Values => ({
  kcal: a.kcal - b.kcal,
  protein: a.protein - b.protein,
  carbs: a.carbs - b.carbs,
  fat: a.fat - b.fat,
})

// roundsTo tells values that round to shown as the server rounds what it
// answers: kcal whole, grams to a tenth, a half up.
const roundsTo = (v: Values, shown: Values) =>
  Math.round(v.kcal) === shown.kcal &&
  (['protein', 'carbs', 'fat'] as const).every((m) => Math.round(v[m] * 10) / 10 === shown[m])

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
// marked with what it brought, the save and the delete at its foot. An
// item's row has its name, what it brings and its amount, and its other
// ways in a menu, so a phone's width leaves the name room to read; the
// day is a button that lists the days by name. onDone closes it;
// onDeleted hears of the meal deleted, which its row goes with.
export function MealEditor({
  meal,
  date,
  today,
  timeZone,
  startHour,
  first,
  onDone,
  onDeleted,
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
  onDeleted?: () => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const what = meal.description || t.toast.aMeal
  const update = useUpdateMeal(date, (answer) => {
    const moved = answer.day.date !== date
    sayChange(
      {
        title: !moved
          ? t.toast.changed(what)
          : answer.day.date === today
            ? t.toast.movedToday
            : t.toast.moved(f.date(answer.day.date, today)),
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
    onDeleted?.()
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
    if (amount === undefined) {
      return undefined
    }
    // A share of a recipe as its fraction, when it is one: 1/3, not 0.333.
    const text = isShare(item, amount.key) ? shareField(amount.n, f.field) : f.field(amount.n)
    return { text, amount, foodId: item.food_id, note: item.note }
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
  // The field of the food being found, and each item's menu and Keep, by
  // the item: the focus goes from one to the other.
  const finder = useRef<HTMLInputElement>(null)
  const menus = useRef(new Map<string, HTMLButtonElement>())
  const keeps = useRef(new Map<string, HTMLButtonElement>())
  // The estimates to save as foods of the user's, a portion of each.
  const [asFoods, setAsFoods] = useState<ReadonlySet<string>>(new Set())
  // What is wrong with each entry that cannot be sent, by its key; and
  // with each item's amount, 0 or more than an item may hold
  // (amountWrong), by the item.
  const [wrongEntries, setWrongEntries] = useState<ReadonlyMap<string, Wrong>>(new Map())
  const [over, setOver] = useState<ReadonlyMap<string, Wrong>>(new Map())
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
  // another food in its place, that food's; an estimate's values typed
  // anew, and the others as they are now (as submit sends them); a
  // food's amount typed anew (as submit sends it), worked out
  // from its food as it is now, as the server works it out; else what it
  // brings now. Undefined while a number typed is none, or a food has not
  // come.
  function nowOf(item: Item): Values | undefined {
    const swap = swaps[item.id]
    if (swap !== undefined) {
      return entryValues(swap, f.parse)
    }
    if (item.food_id === undefined) {
      const opened = estimateOf(item)
      const typed = values.map((v) => {
        const was = f.parse(opened?.texts[v] ?? '')
        const n = f.parse(valueTyping[fieldOf(item, v)] ?? opened?.texts[v] ?? '')
        return n !== undefined && n === was ? item.nutrients[v] : n
      })
      const [kcal, protein, carbs, fat] = typed
      if (values.every((v, i) => typed[i] === item.nutrients[v])) {
        return item.nutrients
      }
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
    const n = worked ?? parseAmount(typing[field] ?? was.text, f.parse)
    const food = foodOf(item.food_id)
    const unitWas = food ? unitIn(food, was.amount.key) : was.amount.key
    const unit = chosen[field] ?? unitWas
    const anew =
      unit !== unitWas ||
      (worked === undefined ? n !== parseAmount(was.text, f.parse) : n !== was.amount.n)
    if (n !== undefined && !anew) {
      return item.nutrients
    }
    return n === undefined || food === undefined ? undefined : nutrientsIn(food, unit, n)
  }
  // wasOf is what an item brought as the meal has it, unrounded as the
  // server sums it where the editor can tell: an item of a food, its
  // food's values in its amount, when they round to the item's (the food
  // is as it was when the item was logged); else the item's values, which
  // come rounded.
  function wasOf(item: Item): Values {
    const food = foodOf(item.food_id)
    const amount = amountOf(item)
    const worked =
      food === undefined || amount === undefined
        ? undefined
        : nutrientsIn(food, unitIn(food, amount.key), amount.n)
    return worked !== undefined && roundsTo(worked, item.nutrients) ? worked : item.nutrients
  }
  // What each item brought (wasOf) and brings now: one left as it was,
  // what it brought; one removed, nothing.
  const items = meal.items.map((item) => {
    const was = wasOf(item)
    const now = removed.has(item.id) ? undefined : (nowOf(item) ?? item.nutrients)
    return { was, now: now === item.nutrients ? was : now, asWas: now === item.nutrients }
  })
  // The total is what the items bring now and those added bring, of the
  // values unrounded, rounded only to be shown. The meal's own is its
  // items' summed unrounded and rounded once; where the editor cannot
  // tell what they brought unrounded, their sum may round to another
  // (186 + 218 + 140 kcal is 544, of a meal of 543.1). Then the
  // difference is kept while an item is as it was, so a meal opened says
  // the day's total, and no change.
  const brought = items.reduce<Values>((sum, i) => plus(sum, i.was), zero)
  const rounding =
    items.some((i) => i.asWas) && !roundsTo(brought, meal.total) ? minus(meal.total, brought) : zero
  const total = [
    ...items.flatMap((i) => (i.now === undefined ? [] : [i.now])),
    ...entries.map((e) => entryValues(e, f.parse) ?? zero),
  ].reduce<Values>(plus, rounding)
  const change = rounded(total.kcal) - rounded(meal.total.kcal)
  // The days a meal is of: today and the 30 before it, and the meal's
  // own, when it is not among them: older, or after a today the app has
  // from before the day's start passed. Left as it is, it is not moved.
  const recent = Array.from({ length: 31 }, (_, i) => addDays(today, -i))
  const days = recent.includes(date) ? recent : date > today ? [date, ...recent] : [...recent, date]
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
    const overs = new Map<string, Wrong>()
    const bad = new Map<string, Wrong>()
    for (const item of meal.items) {
      if (removed.has(item.id)) {
        ops.push({ op: 'remove', item_id: item.id })
        continue
      }
      const estimate = estimateOf(item)
      const swap = swaps[item.id]
      if (swap !== undefined) {
        // Another food in its place, under its id, with its note.
        const checked = entryItem(swap, f.parse)
        if ('wrong' in checked) {
          bad.set(swap.key, checked.wrong)
        } else {
          const { id: _new, ...rest } = checked.item
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
      const n = worked ?? parseAmount(typed, f.parse)
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
          (worked === undefined ? n !== parseAmount(was.text, f.parse) : n !== was.amount.n))
      const why = n === undefined ? undefined : amountWrong(itsFood, unit ?? unitWas, n)
      if (n === undefined) {
        notNumbers.add(item.id)
      } else if (!anew) {
        continue
      } else if (why !== undefined) {
        overs.set(item.id, why)
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
      const checked = entryItem(entry, f.parse)
      if ('wrong' in checked) {
        bad.set(entry.key, checked.wrong)
      } else {
        ops.push({ op: 'add', item: checked.item })
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

  // What the foot says is wrong with an amount or an entry: the first's,
  // of an entry still there.
  const firstWrong = [
    ...over.values(),
    ...[...entries, ...Object.values(swaps)].flatMap((e) => wrongEntries.get(e.key) ?? []),
  ].at(0)

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
          {/* The day is a button with its name, which lists the days to
              choose: today, yesterday, then each by its date, in the
              app's language. */}
          <Select.Root
            name="day"
            defaultValue={date}
            items={days.map((d) => ({ value: d, label: dayName(d) }))}
          >
            <div className="min-w-0 flex-1 space-y-1">
              <Select.Label className={cn(fieldLabel, 'block')}>{t.meal.day}</Select.Label>
              <Select.Trigger className={cn(inputClass, 'flex items-center gap-2 text-left')}>
                <CalendarDays aria-hidden className="size-4 shrink-0 text-muted-foreground" />
                <Select.Value className="min-w-0 flex-1 truncate" />
                <ChevronDown aria-hidden className="size-4 shrink-0 text-muted-foreground" />
              </Select.Trigger>
            </div>
            <Select.Portal>
              <Select.Positioner alignItemWithTrigger={false} sideOffset={4} className="z-50">
                <Select.Popup className="max-h-[min(var(--available-height),22rem)] min-w-(--anchor-width) overflow-y-auto rounded-lg border bg-card p-1 shadow-sheet outline-none">
                  <Select.List>
                    {days.map((d) => (
                      <Select.Item
                        key={d}
                        value={d}
                        className="flex h-11 cursor-default items-center gap-2 rounded-md pr-3 pl-2 text-sm outline-none select-none data-highlighted:bg-muted"
                      >
                        <span className="flex size-4 shrink-0 items-center justify-center">
                          <Select.ItemIndicator>
                            <Check aria-hidden className="size-4" />
                          </Select.ItemIndicator>
                        </span>
                        <Select.ItemText>{dayName(d)}</Select.ItemText>
                      </Select.Item>
                    ))}
                  </Select.List>
                </Select.Popup>
              </Select.Positioner>
            </Select.Portal>
          </Select.Root>
          <label className={cn(f.hour12 ? 'w-28' : 'w-24', 'space-y-1')}>
            <span className={cn(fieldLabel, 'block')}>{t.log.time}</span>
            <TimeInput
              name="time"
              defaultValue={f.clock(openedWith.time)}
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
                // As the time it reads, on the language's clock: 7:30 PM,
                // or 19:30, for 1930.
                const typed = e.currentTarget.value
                const time = parseClock(typed)
                e.currentTarget.value = time === undefined ? typed : f.clock(time)
              }}
            />
          </label>
        </div>
        {night && (
          <p id={`${id}-night`} className="text-xs text-muted-foreground">
            {t.log.nightAfter(f.clock(clock))}
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
            const typedN = parseAmount(typing[field] ?? was?.text ?? '', f.parse) ?? Number.NaN
            const unit = was?.amount.portions
              ? t.meal.portions(typedN)
              : was?.amount.unit === undefined
                ? undefined
                : (recipeUnit(item.source, was.amount.unit, typedN, t) ?? was.amount.unit)
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
                    'space-y-2 py-2',
                    changed && '-mx-2 rounded-md bg-primary-soft/60 px-2',
                  )}
                  hidden={swap !== undefined}
                >
                  {/* Its name and what it brings, its amount, and the rest
                      of its ways in a menu: on a phone's width, the name
                      keeps the room to read. */}
                  <div className="flex items-center gap-2">
                    <span
                      className={cn(
                        'min-w-0 flex-1 wrap-break-word',
                        gone && 'text-muted-foreground line-through',
                      )}
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
                        <span className="block text-xs text-warning">
                          {t.meal.changedElsewhere}
                        </span>
                      )}
                    </span>
                    {was && (
                      <AmountField
                        // Of another food now, the field and its unit start
                        // again.
                        key={was.foodId}
                        // A recipe's share is a fraction, whose / a keyboard
                        // of digits lacks.
                        inputMode={item.source === 'recipe' ? 'text' : 'decimal'}
                        // Hidden, not gone, while the item is to be removed:
                        // kept again, it has what was typed in it.
                        hidden={gone}
                        name={item.id}
                        aria-label={t.meal.amount(name)}
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
                        unit={unit}
                        select={
                          choices && itsFood
                            ? {
                                name: fieldOf(item, 'unit'),
                                'aria-label': t.log.unit(name),
                                // Held by the editor, not the page: a unit
                                // renamed or gone elsewhere never changes it
                                // unseen.
                                value: picked ?? unitWas,
                                onChange: (e) => {
                                  // The amount is worked out anew in the unit
                                  // chosen, when the food says the grams of
                                  // both.
                                  const unitTo = e.currentTarget.value
                                  const amount = e.currentTarget.form?.elements.namedItem(item.id)
                                  setChosen((cs) => ({ ...cs, [field]: unitTo }))
                                  if (!(amount instanceof HTMLInputElement)) {
                                    return
                                  }
                                  // From what a unit chosen worked out, or the
                                  // amount as it was while it is not typed, or
                                  // as it is typed.
                                  const base =
                                    exact[field] ??
                                    (typing[field] === undefined ? was.amount.n : typedN)
                                  const worked = Number.isFinite(base)
                                    ? converted(
                                        itsFood,
                                        chosen[field] ?? unitWas ?? was.amount.key,
                                        unitTo,
                                        base,
                                      )
                                    : undefined
                                  if (worked === undefined) {
                                    // Not to be worked out: the amount is
                                    // typed anew in the unit chosen, not taken
                                    // over in it.
                                    amount.value = ''
                                    setTyping((ts) => ({ ...ts, [field]: '' }))
                                    setExact(({ [field]: _gone, ...es }) => es)
                                  } else {
                                    // Back in its own unit, the amount it was,
                                    // not one a hair off it.
                                    const n =
                                      unitTo === unitWas &&
                                      Math.abs(worked - was.amount.n) <=
                                        1e-9 * Math.max(1, was.amount.n)
                                        ? was.amount.n
                                        : worked
                                    const next = isShare(itsFood, unitTo)
                                      ? shareField(n, f.field)
                                      : shownAmount(n, f.field)
                                    amount.value = next
                                    setTyping((ts) => ({ ...ts, [field]: next }))
                                    setExact((es) => ({ ...es, [field]: n }))
                                  }
                                },
                                children: choices.map((u) => (
                                  <option key={u} value={u}>
                                    {u === 'portion'
                                      ? t.meal.portions(typedN)
                                      : entryUnitLabel(itsFood, u, typedN, t)}
                                  </option>
                                )),
                              }
                            : undefined
                        }
                      />
                    )}
                    {gone ? (
                      <Button
                        ref={(el) => {
                          if (el) {
                            keeps.current.set(item.id, el)
                          }
                          return () => {
                            keeps.current.delete(item.id)
                          }
                        }}
                        variant="ghost"
                        size="icon-sm"
                        aria-label={t.meal.keep(name)}
                        onClick={() => {
                          // Kept, the focus goes back to its menu.
                          flushSync(() => {
                            setRemoved((rs) => new Set([...rs].filter((r) => r !== item.id)))
                          })
                          menus.current.get(item.id)?.focus()
                        }}
                      >
                        <RotateCcw aria-hidden />
                      </Button>
                    ) : (
                      <ItemMenu
                        name={name}
                        asFood={estimate ? asFoods.has(item.id) : undefined}
                        finder={finder}
                        trigger={(el) => {
                          if (el) {
                            menus.current.set(item.id, el)
                          }
                          return () => {
                            menus.current.delete(item.id)
                          }
                        }}
                        onFind={() => {
                          // Its field there before the menu gives it the focus.
                          flushSync(() => {
                            setFinding(item.id)
                          })
                        }}
                        onAsFood={(checked) => {
                          setAsFoods((fs) =>
                            checked
                              ? new Set([...fs, item.id])
                              : new Set([...fs].filter((s) => s !== item.id)),
                          )
                        }}
                        onRemove={() => {
                          // Removed, its Keep takes the focus where its menu
                          // was.
                          flushSync(() => {
                            setRemoved((rs) => new Set([...rs, item.id]))
                          })
                          keeps.current.get(item.id)?.focus()
                        }}
                      />
                    )}
                  </div>
                  {estimate && asFoods.has(item.id) && !gone && (
                    <p className="text-xs text-muted-foreground">{t.meal.asFood}</p>
                  )}
                  {estimate && (
                    <input
                      name={fieldOf(item, 'name')}
                      aria-label={t.meal.value(t.log.estimateName, name)}
                      defaultValue={estimate.name}
                      maxLength={200}
                      autoComplete="off"
                      hidden={gone}
                      className={inputClass}
                    />
                  )}
                  {estimate && (
                    // Hidden, not gone, as an amount's field.
                    <span className="grid grid-cols-4 gap-2" hidden={gone}>
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
                    <FoodPicker
                      label={t.meal.findInPlaceOf(name)}
                      field={finder}
                      onPick={(food) => {
                        const entry = foodEntry(food, f.field)
                        setSwaps((ss) => ({ ...ss, [item.id]: entry }))
                        setFinding(undefined)
                        setAdded(entry.key)
                      }}
                      onCancel={() => {
                        // Back to the menu it came from.
                        menus.current.get(item.id)?.focus()
                        setFinding(undefined)
                      }}
                    />
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
                        // Taken back, the item is as it was, and its menu
                        // takes the focus where the other food was.
                        const { [item.id]: _gone, ...rest } = swaps
                        flushSync(() => {
                          setSwaps(rest)
                        })
                        menus.current.get(item.id)?.focus()
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
            className={cn(linkTarget, 'h-auto p-0 text-small')}
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
        {firstWrong !== undefined && (
          <p id={`${id}-wrong-entry`} role="alert" className="text-xs text-destructive">
            {sayWrong(firstWrong, t, f.number)}
          </p>
        )}
        {(ahead || badTime) && (
          <p id={`${id}-time`} role="alert" className="text-xs text-destructive">
            {badTime ? t.meal.timeFormat(f.clock('19:30')) : t.meal.timeToCome}
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

// The classes of a choice in an item's menu.
const choice =
  'flex h-11 cursor-default items-center gap-2 rounded-md pr-3 pl-2 text-sm outline-none select-none data-highlighted:bg-muted [&_svg]:size-4 [&_svg]:shrink-0'

// ItemMenu is the ways to change an item of a meal but its amount, behind
// its ⋯ button: another food in its place, an estimate saved as a food of
// the user's (asFood, whether it is; undefined for a food's item), or the
// item removed. Another food chosen, the focus goes to the field it is
// found in (finder), there by then; the item removed, nowhere, as its Keep
// takes it (onRemove); otherwise back to the button, which trigger is
// given.
function ItemMenu({
  name,
  asFood,
  finder,
  trigger,
  onFind,
  onAsFood,
  onRemove,
}: {
  name: string
  asFood: boolean | undefined
  finder: RefObject<HTMLInputElement | null>
  trigger: Ref<HTMLButtonElement>
  onFind: () => void
  onAsFood: (checked: boolean) => void
  onRemove: () => void
}) {
  const t = useMessages()
  // What was chosen since the menu opened.
  const chosen = useRef<'find' | 'remove' | undefined>(undefined)

  return (
    <Menu.Root
      onOpenChange={(open) => {
        if (open) {
          chosen.current = undefined
        }
      }}
    >
      <Menu.Trigger
        ref={trigger}
        aria-label={t.meal.more(name)}
        className={buttonVariants({ variant: 'ghost', size: 'icon-sm' })}
      >
        <Ellipsis aria-hidden />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Positioner sideOffset={4} align="end" className="z-50">
          <Menu.Popup
            finalFocus={() =>
              chosen.current === 'find' ? (finder.current ?? true) : chosen.current === undefined
            }
            className="min-w-48 rounded-lg border bg-card p-1 shadow-sheet outline-none"
          >
            <Menu.Item
              aria-label={t.meal.anotherFood(name)}
              className={choice}
              onClick={() => {
                chosen.current = 'find'
                onFind()
              }}
            >
              <ArrowLeftRight aria-hidden />
              {t.meal.anotherFoodShort}
            </Menu.Item>
            {asFood !== undefined && (
              <Menu.CheckboxItem
                aria-label={t.meal.saveAsFood(name)}
                checked={asFood}
                closeOnClick
                onCheckedChange={onAsFood}
                className={choice}
              >
                <BookmarkPlus aria-hidden />
                <span className="flex-1">{t.meal.asFoodShort}</span>
                <Menu.CheckboxItemIndicator>
                  <Check aria-hidden />
                </Menu.CheckboxItemIndicator>
              </Menu.CheckboxItem>
            )}
            <Menu.Item
              aria-label={t.meal.remove(name)}
              className={cn(choice, 'text-destructive')}
              onClick={() => {
                chosen.current = 'remove'
                onRemove()
              }}
            >
              <X aria-hidden />
              {t.meal.removeShort}
            </Menu.Item>
          </Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
  )
}
