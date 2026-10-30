import { Link, useSearch } from '@tanstack/react-router'
import { useId, useRef, useState, type RefObject } from 'react'

import { detailOf, statusOf } from '@/api/errors'
import { recipeInput, useIngredientFoods, useSaveRecipe, type Ingredient } from '@/api/recipes'
import type { Food } from '@/api/types'
import { entryWrong, type FoodEntry } from '@/components/day/meal-entries'
import { SaveStatus } from '@/components/day/save-status'
import { LoadError } from '@/components/load-error'
import { DishSummary } from '@/components/recipes/dish-summary'
import { IngredientRow } from '@/components/recipes/ingredient-row'
import { PortionsField } from '@/components/recipes/portions-field'
import {
  dishFrom,
  dishOf,
  dishWrong,
  entryUnitLabel,
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
import { Sheet } from '@/components/sheet'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { linkTarget } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

// CookedAgain saves a recipe cooked again, in a sheet: the same recipe, a
// new pot, of which only the amounts, the cooked dish's weight and the
// portions change. The weight is weighed anew, helped by the empty pot
// remembered, not taken from the last pot. Its foot says what a portion
// (or 100 g) comes to now, and what it was. weigh only weighs the dish
// as it is: its weight alone.
export function CookedAgain({
  food,
  weigh = false,
  open,
  onOpenChange,
  opener,
}: {
  food: Food
  weigh?: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
  opener?: RefObject<HTMLElement | null>
}) {
  const t = useMessages()
  const done = () => {
    onOpenChange(false)
  }

  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title={weigh ? t.recipes.weighTitle : t.recipes.cookedAgain}
      finalFocus={opener}
      bare
    >
      {weigh ? <Pot food={food} onDone={done} /> : <Ingredients food={food} onDone={done} />}
    </Sheet>
  )
}

// Ingredients are the recipe's ingredients' foods, loaded for their
// amounts, before the pot.
function Ingredients({ food, onDone }: { food: Food; onDone: () => void }) {
  const t = useMessages()
  const f = useFormat()
  const items = food.recipe?.items ?? []
  const foods = useIngredientFoods(items.map((i) => i.food_id))
  if (foods.foods === undefined) {
    return (
      <div className="p-4">
        {foods.error === undefined ? (
          <p className="text-muted-foreground">{t.loading}</p>
        ) : (
          <LoadError title={t.loadFailed.food} error={foods.error} onRetry={foods.retry} />
        )}
      </div>
    )
  }
  const its = foods.foods
  const start = items.flatMap((ingredient) => {
    const of = its.get(ingredient.food_id)
    return of === undefined ? [] : [{ ingredient, entry: ingredientEntry(ingredient, of, f.field) }]
  })
  return <Pot food={food} start={start} onDone={onDone} />
}

// Pot is the recipe cooked again, or weighed (without start, its
// ingredients as they are).
function Pot({
  food,
  start,
  onDone,
}: {
  food: Food
  start?: { ingredient: Ingredient; entry: FoodEntry }[]
  onDone: () => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const search = useSearch({ strict: false })
  const recipe = food.recipe
  const save = useSaveRecipe()
  const [entries, setEntries] = useState(() => start?.map((s) => s.entry) ?? [])
  // How much each was, as the recipe has it, by its entry's key.
  const [was] = useState(
    () =>
      new Map(
        (start ?? []).map(({ ingredient, entry }) => [
          entry.key,
          { amount: entry.amount, unit: entry.unit, note: ingredient.note },
        ]),
      ),
  )
  const [weighing, setWeighing] = useState<Weighing>(() => weighingOf(f.field, recipe?.pot_g))
  const [portions, setPortions] = useState(
    recipe?.portions === undefined ? '' : String(recipe.portions),
  )
  const [tried, setTried] = useState(false)
  const yieldG = yieldOfWeighing(weighing, f.parse)
  const portionCount = start === undefined ? recipe?.portions : portionsOf(portions, f.parse)
  const dish =
    start === undefined
      ? dishFrom(recipe?.total ?? food.nutrients, recipe?.raw_weight, yieldG, portionCount)
      : dishOf(entries, f.parse, yieldG, portionCount)
  const before = recipe?.portion?.kcal ?? food.nutrients.kcal
  const weightWrong = weighingWrong(weighing, f.parse)
  const portionsWrong = portions.trim() !== '' && portionsOf(portions, f.parse) === undefined
  const wrongDish = dishWrong(dish, start === undefined ? 1 : entries.length)
  const status = statusOf(save.error)
  // Weighed alone, a weight is what it saves.
  const unweighed = start === undefined && yieldG === undefined && !weighing.cantWeigh

  const resend = useRef<() => void>(undefined)

  function submit() {
    const notes = new Map<string, string>()
    for (const [key, w] of was) {
      if (w.note !== undefined) {
        notes.set(key, w.note)
      }
    }
    const items = start === undefined ? recipeInput(food).items : itemsOf(entries, notes, f.parse)
    if (
      items === undefined ||
      weightWrong.weight === true ||
      weightWrong.pot === true ||
      portionsWrong ||
      wrongDish !== undefined ||
      unweighed
    ) {
      setTried(true)
      return
    }
    const { yield_g: _was, portions: _portions, pot_g: _pot, ...kept } = recipeInput(food)
    const pot = potOf(weighing, f.parse, recipe?.pot_g)
    // Sent again from the status as it was sent, with what follows a save.
    resend.current = () => {
      save.mutate(
        {
          body: {
            ...kept,
            items,
            ...(yieldG === undefined ? {} : { yield_g: yieldG }),
            ...(portionCount === undefined ? {} : { portions: portionCount }),
            ...(pot === undefined ? {} : { pot_g: pot }),
          },
        },
        {
          onSuccess: (answer) => {
            const r = answer.food.recipe
            const per = r?.portion
            sayChange(
              {
                title: t.recipes.savedPer(
                  answer.food.name,
                  t.unit.kcal(f.number(per?.kcal ?? answer.food.nutrients.kcal)),
                  per ? t.recipes.perPortion : t.recipes.per100g,
                ),
                lines: answer.warnings.map((w) => warningOf(w, t)),
              },
              { last: { action: 'recipe.save', entity: answer.food.id }, date: 'today' },
            )
            onDone()
          },
        },
      )
    }
    resend.current()
  }

  return (
    <>
      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-4">
        <p className="text-sm text-muted-foreground">
          {start === undefined ? t.recipes.weighSheetHint : t.recipes.cookedHint}
        </p>
        {start !== undefined && (
          <section aria-labelledby={`${id}-items`} className="space-y-1">
            <h2 id={`${id}-items`} className="text-h3">
              {t.recipes.ingredientsTitle}
            </h2>
            <ul aria-labelledby={`${id}-items`} className="divide-y">
              {entries.map((entry) => {
                const w = was.get(entry.key)
                const changed =
                  w !== undefined && (w.amount !== entry.amount || w.unit !== entry.unit)
                return (
                  <IngredientRow
                    key={entry.key}
                    entry={entry}
                    wrong={tried ? entryWrong(entry, f.parse) : undefined}
                    was={
                      changed
                        ? `${w.amount} ${entryUnitLabel(entry.food, w.unit, f.parse(w.amount) ?? 1, t)}`
                        : undefined
                    }
                    onChange={(next) => {
                      setEntries((es) => es.map((e) => (e.key === entry.key ? next : e)))
                    }}
                  />
                )
              })}
            </ul>
            <p className="text-small text-muted-foreground">
              {t.recipes.otherIngredients}{' '}
              <Link
                to="/foods/$id"
                params={{ id: food.id }}
                search={{ ...search, edit: true }}
                className={cn(
                  linkTarget,
                  'font-semibold text-primary underline-offset-4 hover:underline',
                )}
                onClick={onDone}
              >
                {t.recipes.editTitle}
              </Link>
            </p>
          </section>
        )}
        <WeightFields
          value={weighing}
          onChange={setWeighing}
          raw={dish.raw}
          remembered={recipe?.pot_g !== undefined && f.parse(weighing.empty) === recipe.pot_g}
          last={
            recipe?.yield_g === undefined
              ? t.recipes.lastNotWeighed
              : t.recipes.lastTime(t.unit.g(f.number(recipe.yield_g)))
          }
          wrong={tried ? weightWrong : {}}
        />
        {start !== undefined && (
          <PortionsField
            value={portions}
            onChange={setPortions}
            weight={dish.weight}
            weighed={dish.weighed}
            wrong={tried && portionsWrong}
          />
        )}
      </div>
      <div className="space-y-2 border-t p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <DishSummary dish={dish} before={before} />
        {tried && wrongDish !== undefined && (
          <p role="alert" className="text-sm text-destructive">
            {wrongDish === 'noRaw'
              ? t.recipes.noRaw
              : t.recipes.rawTooHeavy(f.number(limits.recipe.maxWeight))}
          </p>
        )}
        {tried && unweighed && (
          <p role="alert" className="text-sm text-destructive">
            {t.recipes.enterWeight}
          </p>
        )}
        {save.isError && status === 422 ? (
          <p role="alert" className="text-sm text-destructive">
            {t.recipes.refused(detailOf(save.error))}
          </p>
        ) : (
          <SaveStatus change={save} onRetry={() => resend.current?.()} />
        )}
        <Button className="h-13 w-full text-base" disabled={save.isPending} onClick={submit}>
          {start === undefined ? t.recipes.saveWeight : t.recipes.saveCooked}
        </Button>
      </div>
    </>
  )
}
