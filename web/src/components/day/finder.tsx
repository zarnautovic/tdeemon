import { Tabs } from '@base-ui/react/tabs'
import { Check, Plus, Repeat2, Search } from 'lucide-react'
import { Fragment, useEffect, useId, useRef, useState, type RefObject } from 'react'

import { $api } from '@/api/client'
import { useFindFoods } from '@/api/foods'
import { useRecentMeals } from '@/api/meals'
import type { Food, Meal, RecentMeal } from '@/api/types'
import {
  entryValues,
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
import type { Making } from '@/components/day/new-entry'
import { pastMeals } from '@/components/day/past-meals'
import { suggestFoods, suggestMeals, type Moment } from '@/components/day/suggest'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { useDesktop } from '@/lib/media'
import { matches, parts } from '@/lib/search'
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
// a search (query) finds, or those eaten lately or most, and the meals of
// before; a food picked opens in its row for its amount, in the amount it
// was eaten in last, and a tap on its + adds it so. What it offers first
// is what is eaten at the meal of the day being logged (at, suggestMeals
// and suggestFoods); with nothing searched for, the meals of it come as
// cards above the search, repeated in a tap. A new food or an estimate
// is the sheet's next step (onMake, with the button that asks for it).
// onAdd takes an entry, checked; onRepeat a meal to log again. search is
// the field the sheet's focus goes to.
export function Finder({
  search,
  query,
  onQuery,
  at,
  onAdd,
  onRepeat,
  onMake,
}: {
  search: RefObject<HTMLInputElement | null>
  query: string
  onQuery: (query: string) => void
  at: Moment
  onAdd: (entry: Entry) => void
  onRepeat: (meal: Meal) => void
  onMake: (making: Making, button: HTMLElement) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const [list, setList] = useState<List>('all')
  // The food picked, with its amount, which opens in its row.
  const [picked, setPicked] = useState<FoodEntry | undefined>(undefined)
  const [pickedWrong, setPickedWrong] = useState(false)
  // Two cards side by side on a phone, three on a desktop's sheet.
  const cards = useDesktop() ? 3 : 2
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
  const recentMeals = useRecentMeals()
  const q = query.trim()

  // eaten are the foods of a list eaten before, that the search matches.
  const eaten = (foods: Food[] | undefined) =>
    (foods ?? []).filter(
      (food) =>
        food.last_use !== undefined && matches(q, [food.name, food.brand, ...(food.aliases ?? [])]),
    )
  // The meals the search matches, by their words or their items' names
  // and brands, each once: of the meals of the same foods, the latest it
  // matches; those eaten at the meal of the day lately first.
  const ranked = suggestMeals(
    pastMeals(
      (recentMeals.data?.meals ?? []).filter((m) =>
        matches(q, [m.description, ...m.items.flatMap((i) => [i.name, i.brand])]),
      ),
    ),
    at,
  )
  // Nothing searched for, the first of the meal of the day are cards, which
  // the lists leave out.
  const suggested = q === '' ? ranked.meals.slice(0, Math.min(ranked.ofSlot, cards)) : []
  const meals = ranked.meals.slice(suggested.length)
  const shown: Record<List, { foods: Food[]; meals: Meal[]; pending: boolean }> = {
    all:
      q === ''
        ? {
            foods: suggestFoods(
              eaten(recentFoods.data?.foods),
              recentMeals.data?.meals ?? [],
              at,
            ).slice(0, firstFoods),
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

  // add adds an entry checked.
  function add(entry: FoodEntry) {
    if (itemOf(entry, f.parse) === undefined) {
      setPicked(entry)
      setPickedWrong(true)
      return
    }
    onAdd(entry)
    setPicked(undefined)
    setPickedWrong(false)
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
      {suggested.length > 0 && (
        <section aria-labelledby={`${id}-suggested`} className="space-y-2">
          <h3 id={`${id}-suggested`} className="text-small font-semibold text-muted-foreground">
            {t.log.forSlot[at.slot]}
          </h3>
          <ul className={cn('grid gap-2', cards === 3 ? 'grid-cols-3' : 'grid-cols-2')}>
            {suggested.map((meal) => (
              <Suggestion key={meal.id} meal={meal} onRepeat={onRepeat} />
            ))}
          </ul>
        </section>
      )}
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
            onQuery(e.currentTarget.value)
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
              query={q}
              timeZone={at.timeZone}
              // The cards have said what there is.
              quiet={suggested.length > 0}
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
      {/* Side by side while they fit, their words wrapped. */}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          className="h-auto min-h-11 min-w-0 flex-1 basis-40 border-dashed py-2 whitespace-normal"
          onClick={(e) => {
            onMake('estimate', e.currentTarget)
          }}
        >
          <Plus aria-hidden />
          {t.log.anEstimateOf}
        </Button>
        <Button
          variant="outline"
          className="h-auto min-h-11 min-w-0 flex-1 basis-40 border-dashed py-2 whitespace-normal"
          onClick={(e) => {
            onMake('food', e.currentTarget)
          }}
        >
          <Plus aria-hidden />
          {t.log.newFood}
        </Button>
      </div>
    </div>
  )
}

// Suggestion is a meal eaten lately at the meal of the day being logged,
// as a card: its name, its kcal, and Repeat, which logs it again.
function Suggestion({ meal, onRepeat }: { meal: RecentMeal; onRepeat: (meal: Meal) => void }) {
  const t = useMessages()
  const f = useFormat()
  const name = mealName(meal, t)

  return (
    <li className="flex min-w-0 flex-col gap-2 rounded-lg border bg-card p-3">
      <span className="line-clamp-2 font-medium wrap-break-word">{name}</span>
      <span className="text-caption text-muted-foreground">
        {t.unit.kcal(f.number(meal.total.kcal))}
      </span>
      <Button
        variant="outline"
        size="sm"
        className="mt-auto w-full text-primary"
        aria-label={t.log.repeatOf(name)}
        onClick={() => {
          onRepeat(meal)
        }}
      >
        <Repeat2 aria-hidden />
        {t.log.repeat}
      </Button>
    </li>
  )
}

// Results are a list's foods and meals, or what there is to say when it
// has none, unless it is quiet: what there is was said already.
function Results({
  foods,
  meals,
  pending,
  query,
  timeZone,
  quiet,
  picked,
  pickedRow,
  onPick,
  onAdd,
  onRepeat,
}: {
  foods: Food[]
  meals: Meal[]
  pending: boolean
  // What was searched for, trimmed.
  query: string
  timeZone: string
  quiet: boolean
  picked: string | undefined
  pickedRow: (food: Food) => React.ReactNode
  onPick: (food: Food) => void
  onAdd: (food: Food) => void
  onRepeat: (meal: Meal) => void
}) {
  const t = useMessages()
  const searched = query !== ''
  if (foods.length === 0 && meals.length === 0) {
    if (quiet && !pending) {
      return null
    }
    return (
      <p className="py-2 text-sm text-muted-foreground">
        {pending ? t.loading : searched ? t.log.nothing : t.log.none}
      </p>
    )
  }
  const mealRows = meals.map((meal) => (
    <MealResult key={meal.id} meal={meal} query={query} timeZone={timeZone} onRepeat={onRepeat} />
  ))
  const foodRows = foods.map((food) => (
    <FoodResult
      key={food.id}
      food={food}
      query={query}
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

// Marked is a text with what the search for query matched in it marked,
// as the start of a word.
function Marked({ text, query }: { text: string; query: string }) {
  return parts(query, text).map((p) =>
    p.matched ? (
      <mark key={p.at} className="rounded-xs bg-highlight text-inherit">
        {p.text}
      </mark>
    ) : (
      <Fragment key={p.at}>{p.text}</Fragment>
    ),
  )
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

// FoodResult is a food found, what the search matched in its name
// marked: a tap opens it for its amount (amount), its + adds it in the
// amount it was eaten in last.
function FoodResult({
  food,
  query,
  open,
  amount,
  onPick,
  onAdd,
}: {
  food: Food
  query: string
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
              <Marked text={food.name} query={query} />
              {food.brand && (
                <span className="text-muted-foreground">
                  {' · '}
                  <Marked text={food.brand} query={query} />
                </span>
              )}
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
              {t.log.serving(s.name, unitOfServing(s.amount))}
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
// it was eaten, how many items and its kcal, what the search matched in
// its name marked.
function MealResult({
  meal,
  query,
  timeZone,
  onRepeat,
}: {
  meal: Meal
  query: string
  timeZone: string
  onRepeat: (meal: Meal) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const name = mealName(meal, t)

  return (
    <li className="flex min-h-14 items-center gap-3 px-2 py-2">
      <span
        aria-hidden
        className="grid size-10 shrink-0 place-items-center rounded-md bg-surface-2 text-muted-foreground"
      >
        <Repeat2 className="size-4.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium">
          <Marked text={name} query={query} />
        </span>
        {longName(meal.description) && (
          <span className="block truncate text-caption text-muted-foreground">
            <Marked text={meal.description} query={query} />
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
