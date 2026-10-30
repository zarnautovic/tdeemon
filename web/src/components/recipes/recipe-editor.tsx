import { Menu } from '@base-ui/react/menu'
import { ArrowDown, ArrowUp, Ellipsis, Plus, Trash2 } from 'lucide-react'
import { useId, useRef, useState, type SubmitEvent } from 'react'
import { flushSync } from 'react-dom'

import { conflictOf, detailOf, statusOf } from '@/api/errors'
import {
  useCreateRecipe,
  useIngredientFoods,
  useSaveRecipe,
  type RecipeWarning,
} from '@/api/recipes'
import type { Food } from '@/api/types'
import { entryWrong, type FoodEntry } from '@/components/day/meal-entries'
import { SaveStatus } from '@/components/day/save-status'
import { TakenBy } from '@/components/foods/taken-by'
import { LoadError } from '@/components/load-error'
import { DishSummary } from '@/components/recipes/dish-summary'
import { IngredientRow } from '@/components/recipes/ingredient-row'
import { IngredientSheet } from '@/components/recipes/ingredient-sheet'
import { PortionsField } from '@/components/recipes/portions-field'
import {
  dishOf,
  dishWrong,
  ingredientEntry,
  itemsOf,
  portionsOf,
  warningOf,
} from '@/components/recipes/recipe-values'
import {
  potOf,
  weighingOf,
  weighingWrong,
  yieldOfWeighing,
  type Weighing,
} from '@/components/recipes/weighing'
import { WeightFields } from '@/components/recipes/weight-fields'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { fieldLabel, inputClass } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

// RecipeEditor writes a recipe, a new one or one of the user's (food, a
// recipe's food with its recipe), whose ingredients' foods it loads
// first, as they are now. onSaved gives the recipe's food as saved.
export function RecipeEditor({
  food,
  onSaved,
}: {
  food?: Food | undefined
  onSaved: (food: Food) => void
}) {
  const t = useMessages()
  const items = food?.recipe?.items ?? []
  const foods = useIngredientFoods(items.map((i) => i.food_id))

  if (food === undefined) {
    return <Form onSaved={onSaved} start={[]} notes={new Map()} />
  }
  if (foods.foods === undefined) {
    return foods.error === undefined ? (
      <div
        aria-hidden
        className="h-96 animate-pulse rounded-lg bg-surface-2 motion-reduce:animate-none"
      >
        <span className="sr-only">{t.loading}</span>
      </div>
    ) : (
      <LoadError title={t.loadFailed.food} error={foods.error} onRetry={foods.retry} />
    )
  }
  return <Started food={food} foods={foods.foods} onSaved={onSaved} />
}

// Started is a recipe's editor, its ingredients' foods come.
function Started({
  food,
  foods,
  onSaved,
}: {
  food: Food
  foods: ReadonlyMap<string, Food>
  onSaved: (food: Food) => void
}) {
  const f = useFormat()
  // Made once: the editor keeps what is typed, not what comes later.
  const [start] = useState(() => {
    const entries: FoodEntry[] = []
    const notes = new Map<string, string>()
    for (const ingredient of food.recipe?.items ?? []) {
      const its = foods.get(ingredient.food_id)
      if (its !== undefined) {
        const entry = ingredientEntry(ingredient, its, f.field)
        entries.push(entry)
        if (ingredient.note !== undefined) {
          notes.set(entry.key, ingredient.note)
        }
      }
    }
    return { entries, notes }
  })
  return <Form food={food} start={start.entries} notes={start.notes} onSaved={onSaved} />
}

// Form is the recipe as typed: its name; its ingredients, added in a sheet
// (IngredientSheet), each in its amount, moved or removed in its menu; the
// cooked dish's weight (WeightFields) and its portions (PortionsField),
// both optional; and a note. Its foot, in view as the page scrolls, says
// what the recipe comes to as it is typed (DishSummary), and saves it.
// What is wrong is said at its field once a save was asked for; a save
// the server warns of (a weight far off its raw ingredients') is saved,
// and its toast says why, with Undo.
function Form({
  food,
  start,
  notes,
  onSaved,
}: {
  food?: Food
  start: FoodEntry[]
  notes: ReadonlyMap<string, string>
  onSaved: (food: Food) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const recipe = food?.recipe
  const create = useCreateRecipe()
  const correct = useSaveRecipe()
  const save = food === undefined ? create : correct
  const [name, setName] = useState(food?.name ?? '')
  const [entries, setEntries] = useState(start)
  const [weighing, setWeighing] = useState<Weighing>(() =>
    weighingOf(f.field, recipe?.pot_g, recipe?.yield_g),
  )
  const [portions, setPortions] = useState(
    recipe?.portions === undefined ? '' : String(recipe.portions),
  )
  const [note, setNote] = useState(food?.note ?? '')
  // Whether a save was asked for: what is wrong is said from then on.
  const [tried, setTried] = useState(false)
  const [adding, setAdding] = useState(false)
  const form = useRef<HTMLFormElement>(null)
  const addButton = useRef<HTMLButtonElement>(null)

  const yieldG = yieldOfWeighing(weighing, f.parse)
  const portionCount = portionsOf(portions, f.parse)
  const dish = dishOf(entries, f.parse, yieldG, portionCount)
  const wrongs = new Map(entries.map((e) => [e.key, entryWrong(e, f.parse)]))
  const weightWrong = weighingWrong(weighing, f.parse)
  const wrong = {
    name: name.trim() === '',
    items: entries.length === 0,
    amounts: [...wrongs.values()].some((w) => w !== undefined),
    portions: portions.trim() !== '' && portionCount === undefined,
    weight: weightWrong.weight === true || weightWrong.pot === true,
    dish: dishWrong(dish, entries.length),
  }
  const status = statusOf(save.error)
  const conflict = conflictOf(save.error)
  const most = limits.recipe.maxItems

  // saved says a recipe saved, with what the server warned of it, and
  // Undo.
  const saved = (answer: { food: Food; warnings: RecipeWarning[] }) => {
    sayChange(
      {
        title: t.toast.savedOf(answer.food.name),
        lines: answer.warnings.map((w) => warningOf(w, t)),
      },
      { last: { action: 'recipe.save', entity: answer.food.id }, date: 'today' },
    )
    onSaved(answer.food)
  }

  const resend = useRef<() => void>(undefined)

  function submit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault()
    const items = itemsOf(entries, notes, f.parse)
    if (
      items === undefined ||
      wrong.name ||
      wrong.items ||
      wrong.portions ||
      wrong.weight ||
      wrong.dish !== undefined
    ) {
      flushSync(() => {
        setTried(true)
      })
      form.current?.querySelector<HTMLElement>('[aria-invalid="true"], [data-wrong]')?.focus()
      return
    }
    const pot = potOf(weighing, f.parse, recipe?.pot_g)
    const fields = {
      name: name.trim(),
      ...(food?.aliases === undefined || food.aliases.length === 0
        ? {}
        : { aliases: food.aliases }),
      items,
      ...(yieldG === undefined ? {} : { yield_g: yieldG }),
      ...(portionCount === undefined ? {} : { portions: portionCount }),
      ...(pot === undefined ? {} : { pot_g: pot }),
      ...(note.trim() === '' ? {} : { note: note.trim() }),
    }
    // Sent again from the status as it was sent, with what follows a save.
    resend.current =
      food === undefined
        ? () => {
            create.mutate({ body: fields }, { onSuccess: saved })
          }
        : () => {
            correct.mutate(
              { body: { id: food.id, ...fields, ...(food.archived ? { archived: true } : {}) } },
              { onSuccess: saved },
            )
          }
    resend.current()
  }

  function move(key: string, by: number) {
    setEntries((es) => {
      const i = es.findIndex((e) => e.key === key)
      const j = i + by
      const a = es[i]
      const b = es[j]
      if (a === undefined || b === undefined) {
        return es
      }
      const next = [...es]
      next[i] = b
      next[j] = a
      return next
    })
  }

  function remove(key: string) {
    flushSync(() => {
      setEntries((es) => es.filter((e) => e.key !== key))
    })
    addButton.current?.focus()
  }

  let refused
  if (conflict) {
    refused = (
      <TakenBy foodId={conflict.foodId} by={conflict.alias === undefined ? 'name' : 'alias'} />
    )
  } else if (save.isError && (status === 409 || status === 422)) {
    refused = t.recipes.refused(detailOf(save.error))
  }

  return (
    <>
      <form ref={form} noValidate onSubmit={submit} className="space-y-3">
        <div className="space-y-5 rounded-lg bg-card p-4 shadow-card lg:p-6">
          <div className="space-y-1">
            <label htmlFor={`${id}-name`} className={cn(fieldLabel, 'block')}>
              {t.foods.name}
            </label>
            <input
              id={`${id}-name`}
              value={name}
              maxLength={200}
              autoComplete="off"
              placeholder={t.recipes.nameHint}
              aria-invalid={(tried && wrong.name) || conflict !== undefined}
              aria-describedby={`${id}-name-wrong`}
              className={inputClass}
              onChange={(e) => {
                setName(e.currentTarget.value)
              }}
            />
            <p id={`${id}-name-wrong`} className="text-sm text-destructive empty:hidden">
              {conflict ? refused : tried && wrong.name ? t.foods.needed : null}
            </p>
          </div>
          <section aria-labelledby={`${id}-items`} className="space-y-2">
            <div className="flex items-baseline justify-between gap-2">
              <h2 id={`${id}-items`} className="text-h3">
                {t.recipes.ingredientsTitle}
              </h2>
              <span className="text-small text-muted-foreground">
                {t.recipes.count(f.number(entries.length), f.number(most))}
              </span>
            </div>
            {entries.length === 0 ? (
              <p className="text-small text-muted-foreground">{t.recipes.noIngredients}</p>
            ) : (
              <ul aria-labelledby={`${id}-items`} className="divide-y">
                {entries.map((entry, i) => (
                  <IngredientRow
                    key={entry.key}
                    entry={entry}
                    wrong={tried ? wrongs.get(entry.key) : undefined}
                    onChange={(changed) => {
                      setEntries((es) => es.map((e) => (e.key === entry.key ? changed : e)))
                    }}
                  >
                    <RowMenu
                      name={entry.food.name}
                      first={i === 0}
                      last={i === entries.length - 1}
                      onMove={(by) => {
                        move(entry.key, by)
                      }}
                      onRemove={() => {
                        remove(entry.key)
                      }}
                    />
                  </IngredientRow>
                ))}
              </ul>
            )}
            {tried && wrong.items && (
              <p data-wrong tabIndex={-1} className="text-sm text-destructive outline-none">
                {t.recipes.needsIngredient}
              </p>
            )}
            <Button
              ref={addButton}
              variant="outline"
              className="w-full border-dashed font-semibold text-primary"
              disabled={entries.length >= most}
              focusableWhenDisabled
              onClick={() => {
                setAdding(true)
              }}
            >
              <Plus aria-hidden />
              {t.recipes.addIngredients}
            </Button>
            {entries.length >= most && (
              <p className="text-small text-muted-foreground">
                {t.recipes.mostIngredients(f.number(most))}
              </p>
            )}
            {entries.length > 0 && (
              <p className="flex justify-between gap-2 text-small text-muted-foreground">
                <span>{t.recipes.rawSum}</span>
                <span>
                  {dish.raw === undefined ? '—' : t.unit.g(f.number(dish.raw))} ·{' '}
                  {t.unit.kcal(f.number(dish.total.kcal))}
                </span>
              </p>
            )}
          </section>
          <hr />
          <WeightFields
            value={weighing}
            onChange={setWeighing}
            raw={dish.raw}
            remembered={recipe?.pot_g !== undefined && f.parse(weighing.empty) === recipe.pot_g}
            wrong={tried ? weightWrong : {}}
          />
          {tried && wrong.dish !== undefined && (
            <p data-wrong tabIndex={-1} className="text-sm text-destructive outline-none">
              {wrong.dish === 'noRaw'
                ? t.recipes.noRaw
                : t.recipes.rawTooHeavy(f.number(limits.recipe.maxWeight))}
            </p>
          )}
          <hr />
          <PortionsField
            value={portions}
            onChange={setPortions}
            weight={dish.weight}
            weighed={dish.weighed}
            wrong={tried && wrong.portions}
          />
          <div className="space-y-1">
            <label htmlFor={`${id}-note`} className={cn(fieldLabel, 'block')}>
              {t.foods.note} · {t.recipes.optional}
            </label>
            <textarea
              id={`${id}-note`}
              value={note}
              rows={2}
              maxLength={2000}
              placeholder={t.recipes.noteHint}
              className={cn(inputClass, 'h-auto py-2')}
              onChange={(e) => {
                setNote(e.currentTarget.value)
              }}
            />
          </div>
          <p className="text-caption text-muted-foreground">{t.recipes.savesNow}</p>
        </div>
        {/* In view as the page scrolls: above a phone's bar of sections. */}
        <div className="sticky bottom-[calc(4.5rem+env(safe-area-inset-bottom))] z-10 space-y-2 rounded-lg border bg-card p-3 shadow-sheet lg:bottom-0">
          <DishSummary dish={dish} />
          {!conflict && refused !== undefined ? (
            <p role="alert" className="text-sm text-destructive">
              {refused}
            </p>
          ) : // Of the one the form sends: a new recipe, or one corrected.
          food === undefined ? (
            <SaveStatus change={create} onRetry={() => resend.current?.()} />
          ) : (
            <SaveStatus change={correct} onRetry={() => resend.current?.()} />
          )}
          <Button type="submit" className="h-13 w-full text-base" disabled={save.isPending}>
            {t.recipes.save}
          </Button>
        </div>
      </form>
      <IngredientSheet
        open={adding}
        onOpenChange={setAdding}
        opener={addButton}
        recipe={name}
        exclude={food?.id}
        count={entries.length}
        onAdd={(entry) => {
          setEntries((es) => [...es, entry])
        }}
        onRemove={(key) => {
          setEntries((es) => es.filter((e) => e.key !== key))
        }}
      />
    </>
  )
}

// RowMenu is an ingredient's menu: moved up or down, or removed.
function RowMenu({
  name,
  first,
  last,
  onMove,
  onRemove,
}: {
  name: string
  first: boolean
  last: boolean
  onMove: (by: number) => void
  onRemove: () => void
}) {
  const t = useMessages()
  const item =
    'flex h-11 cursor-default items-center gap-2 rounded-md px-3 text-sm outline-none select-none data-disabled:opacity-45 data-highlighted:bg-muted'

  return (
    <Menu.Root>
      <Menu.Trigger
        aria-label={t.meal.more(name)}
        className="grid size-11 shrink-0 place-items-center rounded-md text-muted-foreground outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Ellipsis aria-hidden className="size-4.5" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Positioner sideOffset={4} align="end" className="z-40">
          <Menu.Popup className="min-w-44 rounded-lg border bg-card p-1 shadow-sheet outline-none">
            <Menu.Item
              className={item}
              disabled={first}
              onClick={() => {
                onMove(-1)
              }}
            >
              <ArrowUp aria-hidden className="size-4" />
              {t.recipes.moveUp}
            </Menu.Item>
            <Menu.Item
              className={item}
              disabled={last}
              onClick={() => {
                onMove(1)
              }}
            >
              <ArrowDown aria-hidden className="size-4" />
              {t.recipes.moveDown}
            </Menu.Item>
            <Menu.Item className={cn(item, 'text-destructive')} onClick={onRemove}>
              <Trash2 aria-hidden className="size-4" />
              {t.meal.removeShort}
            </Menu.Item>
          </Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
  )
}
