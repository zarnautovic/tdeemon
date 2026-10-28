import { Tabs } from '@base-ui/react/tabs'
import { ArrowLeft, Check, Plus, Repeat2, Search } from 'lucide-react'
import { useEffect, useId, useRef, useState, type RefObject } from 'react'

import { $api } from '@/api/client'
import { useFindFoods } from '@/api/foods'
import type { Food, Meal } from '@/api/types'
import { EntryRow } from '@/components/day/entry-row'
import {
  entryValues,
  estimateEntry,
  foodEntry,
  inUnit,
  itemOf,
  lastAmountOf,
  most,
  servingOf,
  unitsOf,
  type Entry,
  type FoodEntry,
} from '@/components/day/meal-entries'
import { longName, mealName } from '@/components/day/meal-slot'
import { FoodForm } from '@/components/foods/food-form'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { matches } from '@/lib/search'
import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// The lists a meal's foods are found in.
const lists = ['all', 'recent', 'frequent', 'meals'] as const
type List = (typeof lists)[number]

function isList(value: unknown): value is List {
  return lists.some((l) => l === value)
}

// How many of the recent meals and foods the first list shows, with
// nothing searched for.
const firstMeals = 3
const firstFoods = 12

// Finder finds what goes in a meal being logged: the foods of the user's
// a search finds, or those eaten lately or most, and the meals of before;
// a food picked opens in its row for its amount, in the amount it was
// eaten in last, and a tap on its + adds it so. A new food is made here
// too, and an estimate given. onAdd takes an entry, checked; onRepeat a
// meal to log again. search is the field the sheet's focus goes to.
export function Finder({
  search,
  timeZone,
  onAdd,
  onRepeat,
}: {
  search: RefObject<HTMLInputElement | null>
  timeZone: string
  onAdd: (entry: Entry) => void
  onRepeat: (meal: Meal) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const [query, setQuery] = useState('')
  const [list, setList] = useState<List>('all')
  // The food picked, with its amount, which opens in its row.
  const [picked, setPicked] = useState<FoodEntry | undefined>(undefined)
  const [pickedWrong, setPickedWrong] = useState(false)
  const [making, setMaking] = useState<'estimate' | 'food' | undefined>(undefined)
  const [estimate, setEstimate] = useState<Entry>(estimateEntry)
  const [estimateWrong, setEstimateWrong] = useState(false)
  const found = useFindFoods(query)
  const recentFoods = $api.useQuery('get', '/api/v1/foods/list', {
    params: { query: { order: 'use', limit: 50 } },
  })
  const frequentFoods = $api.useQuery(
    'get',
    '/api/v1/foods/list',
    { params: { query: { order: 'frequent', limit: 50 } } },
    { enabled: list === 'frequent' },
  )
  const recentMeals = $api.useQuery('get', '/api/v1/meals/recent')
  const q = query.trim()

  // eaten are the foods of a list eaten before, that the search matches.
  const eaten = (foods: Food[] | undefined) =>
    (foods ?? []).filter(
      (food) =>
        food.last_use !== undefined && matches(q, [food.name, food.brand, ...(food.aliases ?? [])]),
    )
  const meals = (recentMeals.data?.meals ?? []).filter((m) =>
    matches(q, [m.description, ...m.items.map((i) => i.name)]),
  )
  const shown: Record<List, { foods: Food[]; meals: Meal[]; pending: boolean }> = {
    all:
      q === ''
        ? {
            foods: eaten(recentFoods.data?.foods).slice(0, firstFoods),
            meals: meals.slice(0, firstMeals),
            pending: recentFoods.isPending || recentMeals.isPending,
          }
        : {
            foods: found.data?.foods ?? [],
            meals: meals.slice(0, firstMeals),
            pending: found.isPending,
          },
    recent: { foods: eaten(recentFoods.data?.foods), meals: [], pending: recentFoods.isPending },
    frequent: {
      foods: eaten(frequentFoods.data?.foods),
      meals: [],
      pending: frequentFoods.isPending,
    },
    meals: { foods: [], meals, pending: recentMeals.isPending },
  }

  function pick(food: Food) {
    setPicked(foodEntry(food, f.field))
    setPickedWrong(false)
  }

  // add adds an entry checked, and goes back to the search for the next.
  function add(entry: FoodEntry) {
    if (itemOf(entry, f.parse) === undefined) {
      setPicked(entry)
      setPickedWrong(true)
      return
    }
    onAdd(entry)
    setPicked(undefined)
    setPickedWrong(false)
    setQuery('')
    search.current?.focus()
  }

  if (making === 'food') {
    return (
      <div className="space-y-3">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            setMaking(undefined)
          }}
        >
          <ArrowLeft aria-hidden />
          {t.log.back}
        </Button>
        <h3 className="text-h3">{t.log.newFood}</h3>
        <FoodForm
          onSaved={(food) => {
            setMaking(undefined)
            pick(food)
          }}
        />
      </div>
    )
  }

  const pickedRow = (food: Food) =>
    picked?.food.id === food.id ? (
      <Amount
        entry={picked}
        wrong={pickedWrong}
        onChange={(e) => {
          setPicked(e)
          setPickedWrong(false)
        }}
        onAdd={add}
      />
    ) : undefined

  return (
    <div className="space-y-3">
      <label className="flex h-12 items-center gap-2.5 rounded-lg border-2 border-input bg-card px-3.5 focus-within:border-ring">
        <Search aria-hidden className="size-4.5 shrink-0 text-muted-foreground" />
        <span className="sr-only">{t.log.search}</span>
        <input
          ref={search}
          type="search"
          value={query}
          placeholder={t.log.findHint}
          autoComplete="off"
          className="h-full min-w-0 flex-1 bg-transparent text-base outline-none"
          onChange={(e) => {
            setQuery(e.currentTarget.value)
          }}
          onKeyDown={(e) => {
            // Enter opens the first food found, for its amount; it sends
            // nothing.
            if (e.key === 'Enter') {
              e.preventDefault()
              const first = shown[list].foods[0]
              if (first !== undefined) {
                pick(first)
              }
            }
          }}
        />
      </label>
      {/* A food made here, which no list has yet. */}
      {picked !== undefined && !shown[list].foods.some((food) => food.id === picked.food.id) && (
        <div className="rounded-lg border p-3">
          <p className="font-medium">{picked.food.name}</p>
          {pickedRow(picked.food)}
        </div>
      )}
      <Tabs.Root
        value={list}
        onValueChange={(value) => {
          if (isList(value)) {
            setList(value)
          }
        }}
      >
        <Tabs.List aria-label={t.log.lists} className="flex gap-1.5">
          {lists.map((l) => (
            <Tabs.Tab
              key={l}
              value={l}
              // 32 px to the eye, 44 to a finger.
              className="relative h-8 min-w-11 rounded-full bg-surface-2 px-3 text-small outline-none after:absolute after:inset-x-0 after:-inset-y-1.5 after:content-[''] focus-visible:ring-2 focus-visible:ring-ring data-active:bg-foreground data-active:font-semibold data-active:text-background"
            >
              {t.log.list[l]}
            </Tabs.Tab>
          ))}
        </Tabs.List>
        {lists.map((l) => (
          <Tabs.Panel
            key={l}
            value={l}
            className="rounded-md pt-2 outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Results
              {...shown[l]}
              searched={q !== ''}
              timeZone={timeZone}
              picked={picked?.food.id}
              pickedRow={pickedRow}
              onPick={(food) => {
                if (picked?.food.id === food.id) {
                  setPicked(undefined)
                } else {
                  pick(food)
                }
              }}
              onAdd={(food) => {
                add(foodEntry(food, f.field))
              }}
              onRepeat={onRepeat}
            />
          </Tabs.Panel>
        ))}
      </Tabs.Root>
      {making === 'estimate' && (
        <div className="space-y-2 rounded-lg border p-3">
          <ul>
            <EntryRow
              entry={estimate}
              wrong={estimateWrong}
              describedBy={estimateWrong ? `${id}-wrong` : undefined}
              focus
              onChange={(e) => {
                setEstimate(e)
              }}
              onRemove={() => {
                setMaking(undefined)
                setEstimateWrong(false)
              }}
            />
          </ul>
          {estimateWrong && (
            <p id={`${id}-wrong`} role="alert" className="text-sm text-destructive">
              {t.log.wrong(most(f.number))}
            </p>
          )}
          <Button
            variant="outline"
            onClick={() => {
              if (itemOf(estimate, f.parse) === undefined) {
                setEstimateWrong(true)
                return
              }
              onAdd(estimate)
              setEstimate(estimateEntry())
              setEstimateWrong(false)
              setMaking(undefined)
              search.current?.focus()
            }}
          >
            <Check aria-hidden />
            {t.log.addOf(
              estimate.kind === 'estimate' ? estimate.name.trim() || t.log.anEstimate : '',
            )}
          </Button>
        </div>
      )}
      {/* Side by side while they fit, their words wrapped. */}
      <div className="flex flex-wrap gap-2">
        {making === undefined && (
          <Button
            variant="outline"
            className="h-auto min-h-11 min-w-0 flex-1 basis-40 border-dashed py-2 whitespace-normal"
            onClick={() => {
              setMaking('estimate')
            }}
          >
            <Plus aria-hidden />
            {t.log.anEstimateOf}
          </Button>
        )}
        <Button
          variant="outline"
          className="h-auto min-h-11 min-w-0 flex-1 basis-40 border-dashed py-2 whitespace-normal"
          onClick={() => {
            setMaking('food')
          }}
        >
          <Plus aria-hidden />
          {t.log.newFood}
        </Button>
      </div>
    </div>
  )
}

// Results are a list's foods and meals, or what there is to say when it
// has none.
function Results({
  foods,
  meals,
  pending,
  searched,
  timeZone,
  picked,
  pickedRow,
  onPick,
  onAdd,
  onRepeat,
}: {
  foods: Food[]
  meals: Meal[]
  pending: boolean
  searched: boolean
  timeZone: string
  picked: string | undefined
  pickedRow: (food: Food) => React.ReactNode
  onPick: (food: Food) => void
  onAdd: (food: Food) => void
  onRepeat: (meal: Meal) => void
}) {
  const t = useMessages()
  if (foods.length === 0 && meals.length === 0) {
    return (
      <p className="py-2 text-sm text-muted-foreground">
        {pending ? t.loading : searched ? t.log.nothing : t.log.none}
      </p>
    )
  }
  const mealRows = meals.map((meal) => (
    <MealResult key={meal.id} meal={meal} timeZone={timeZone} onRepeat={onRepeat} />
  ))
  const foodRows = foods.map((food) => (
    <FoodResult
      key={food.id}
      food={food}
      open={picked === food.id}
      amount={pickedRow(food)}
      onPick={() => {
        onPick(food)
      }}
      onAdd={() => {
        onAdd(food)
      }}
    />
  ))
  return (
    <ul aria-label={t.log.results} className="space-y-0.5">
      {/* Nothing searched for, the meals of before come first: one is
          repeated in a tap. */}
      {searched ? [...foodRows, ...mealRows] : [...mealRows, ...foodRows]}
    </ul>
  )
}

// nameOf is a food's name, with its brand, which tells two of one name
// apart.
function nameOf(food: Food): string {
  return food.brand ? `${food.name} · ${food.brand}` : food.name
}

// initials are a name's first letters, of its first two words.
function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter((w) => w !== '')
    .slice(0, 2)
    .map((w) => w.slice(0, 1).toUpperCase())
    .join('')
}

// FoodResult is a food found: a tap opens it for its amount (amount),
// its + adds it in the amount it was eaten in last.
function FoodResult({
  food,
  open,
  amount,
  onPick,
  onAdd,
}: {
  food: Food
  open: boolean
  amount: React.ReactNode
  onPick: () => void
  onAdd: () => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const lastAmount = lastAmountOf(food, t, f)

  return (
    <li className={cn('rounded-lg', open && 'bg-surface-2/60')}>
      <div className="flex items-center gap-1 pr-1">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={`${id}-amount`}
          className="flex min-h-14 min-w-0 flex-1 items-center gap-3 rounded-lg px-2 py-2 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
          onClick={onPick}
        >
          <span
            aria-hidden
            className="grid size-10 shrink-0 place-items-center rounded-md bg-primary-soft text-small font-semibold text-primary"
          >
            {initials(food.name)}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">
              {food.name}
              {food.brand && <span className="text-muted-foreground"> · {food.brand}</span>}
            </span>
            <span className="block text-caption text-muted-foreground">
              {t.unit.kcal(f.number(food.nutrients.kcal))} {t.foods.per[food.basis]}
              {lastAmount !== undefined && ` · ${t.log.lastAmount(lastAmount)}`}
            </span>
          </span>
        </button>
        {!open && (
          <Button
            variant="outline"
            size="icon"
            className="shrink-0 text-primary"
            aria-label={t.log.addOf(nameOf(food))}
            onClick={onAdd}
          >
            <Plus aria-hidden />
          </Button>
        )}
      </div>
      {open && (
        <div id={`${id}-amount`} className="px-2 pb-3">
          {amount}
        </div>
      )}
    </li>
  )
}

// Amount is a food picked, in an amount to add: typed, in a unit of the
// food's, which works the amount out anew, or one of its servings in a
// tap; with what it brings.
function Amount({
  entry,
  wrong,
  onChange,
  onAdd,
}: {
  entry: FoodEntry
  wrong: boolean
  onChange: (entry: FoodEntry) => void
  onAdd: (entry: FoodEntry) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const field = useRef<HTMLInputElement>(null)
  useEffect(() => {
    field.current?.focus()
  }, [])
  const food = entry.food
  const name = nameOf(food)
  const values = entryValues(entry, f.parse)
  // A food by the portion takes no serving.
  const servings = food.basis === 'portion' ? [] : (food.servings ?? []).slice(0, 3)
  const unitOfServing = (grams: number) =>
    food.basis === '100ml' ? t.unit.ml(f.number(grams)) : t.unit.g(f.number(grams))

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <input
          ref={field}
          aria-label={t.log.amount(name)}
          aria-invalid={wrong}
          aria-describedby={wrong ? `${id}-wrong` : undefined}
          value={entry.amount}
          inputMode="decimal"
          autoComplete="off"
          className={cn(inputClass, 'w-24 tabular-nums')}
          onChange={(e) => {
            // Typed: what a unit chosen worked out is no more.
            const { exact: _typed, ...rest } = entry
            onChange({ ...rest, amount: e.currentTarget.value })
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              onAdd(entry)
            }
          }}
        />
        <select
          aria-label={t.log.unit(name)}
          value={entry.unit}
          className={cn(inputClass, 'w-auto max-w-36')}
          onChange={(e) => {
            // The amount is worked out anew in the unit chosen, so the item
            // weighs what it did.
            onChange(inUnit(entry, e.currentTarget.value, f.parse, f.field))
          }}
        >
          {unitsOf(food).map((u) => (
            <option key={u} value={u}>
              {u === 'portion'
                ? t.log.portions(f.parse(entry.amount) ?? Number.NaN)
                : (servingOf(u) ?? u)}
            </option>
          ))}
        </select>
        <Button
          size="icon"
          className="ml-auto"
          aria-label={t.log.addOf(name)}
          onClick={() => {
            onAdd(entry)
          }}
        >
          <Check aria-hidden />
        </Button>
      </div>
      {servings.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {servings.map((s) => (
            <Button
              key={s.name}
              variant="outline"
              size="sm"
              onClick={() => {
                const { exact: _was, ...rest } = entry
                onChange({ ...rest, unit: `serving:${s.name}`, amount: f.field(1) })
              }}
            >
              {t.log.serving(f.number(1), s.name, unitOfServing(s.amount))}
            </Button>
          ))}
        </div>
      )}
      <p aria-live="polite" className="text-caption text-muted-foreground">
        {values !== undefined &&
          `= ${t.unit.kcal(f.number(values.kcal))} · ${t.day.short.protein} ${f.grams(values.protein)} · ${t.day.short.carbs} ${f.grams(values.carbs)} · ${t.day.short.fat} ${f.grams(values.fat)}`}
      </p>
      {wrong && (
        <p id={`${id}-wrong`} role="alert" className="text-sm text-destructive">
          {t.log.wrong(most(f.number))}
        </p>
      )}
    </div>
  )
}

// MealResult is a meal of before, which a tap logs again: its name, when
// it was eaten, how many items and its kcal.
function MealResult({
  meal,
  timeZone,
  onRepeat,
}: {
  meal: Meal
  timeZone: string
  onRepeat: (meal: Meal) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const name = mealName(meal, timeZone, t.meal.slots)

  return (
    <li className="flex min-h-14 items-center gap-3 px-2 py-2">
      <span
        aria-hidden
        className="grid size-10 shrink-0 place-items-center rounded-md bg-surface-2 text-muted-foreground"
      >
        <Repeat2 className="size-4.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium">{name}</span>
        {longName(meal.description) && (
          <span className="block truncate text-caption text-muted-foreground">
            {meal.description}
          </span>
        )}
        <span className="block text-caption text-muted-foreground">
          {t.log.mealLine(
            `${f.day(meal.eaten_at, timeZone)} ${f.time(meal.eaten_at, timeZone)}`,
            t.meal.items(meal.items.length),
            t.unit.kcal(f.number(meal.total.kcal)),
          )}
        </span>
      </span>
      <Button
        variant="outline"
        size="sm"
        className="shrink-0 text-primary"
        aria-label={t.log.repeatOf(name)}
        onClick={() => {
          onRepeat(meal)
        }}
      >
        {t.log.repeat}
      </Button>
    </li>
  )
}
