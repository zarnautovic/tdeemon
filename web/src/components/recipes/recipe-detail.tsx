import { useQuery } from '@tanstack/react-query'
import { Link, useSearch } from '@tanstack/react-router'
import {
  Archive,
  ArchiveRestore,
  ChevronRight,
  CookingPot,
  Info,
  Pencil,
  RefreshCw,
  Scale,
} from 'lucide-react'
import { useId, useRef, useState } from 'react'

import { fieldsOf, useFoodUse, useSaveFood } from '@/api/foods'
import { recipeInput, useSaveRecipe, type RecipeBody } from '@/api/recipes'
import { sessionQuery } from '@/api/session'
import type { Food } from '@/api/types'
import { SaveStatus } from '@/components/day/save-status'
import { EnergySplit } from '@/components/foods/energy-split'
import { FoodAvatar } from '@/components/foods/food-avatar'
import { LogFood } from '@/components/foods/log-food'
import { NutritionTable } from '@/components/foods/nutrition-table'
import { CookedAgain } from '@/components/recipes/cooked-again'
import { ingredientAmount, warningOf } from '@/components/recipes/recipe-values'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

// A card of the page, as a food's.
const card = 'rounded-lg bg-card p-4 shadow-card lg:p-5'

// RecipeDetail is a recipe's food, read: its name, its ingredients and
// portions; a portion's energy (or 100 g's) and where it comes from, 100
// g's and the whole dish's; whether the dish was weighed, said plainly
// (≈ on its grams, and on what is per gram, when not, never on a portion
// or a share); its ingredients, each as it was given, with what it
// brought, one whose food changed since marked, with a way to take the
// new values (the recipe saved again); what it brings (NutritionTable);
// its note, and how often it was logged. It logs the recipe today, saves
// it cooked again or weighed (CookedAgain), opens its editor, and
// archives it or brings it back, which a toast says, with Undo.
export function RecipeDetail({ food, recipe }: { food: Food; recipe: RecipeBody }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const search = useSearch({ from: '/app/foods/$id' })
  const use = useFoodUse(food.id)
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const archive = useSaveFood()
  const again = useSaveRecipe()
  const [sheet, setSheet] = useState<'cooked' | 'weigh' | undefined>(undefined)
  const cookedButton = useRef<HTMLButtonElement>(null)
  const weighButton = useRef<HTMLButtonElement>(null)
  const last = use.data?.last_use
  const eaten: Food = last === undefined ? food : { ...food, last_use: last }
  const items = recipe.items ?? []
  const changed = items.filter((i) => i.changed === true)
  const weighed = recipe.yield_g !== undefined
  const approx = weighed ? '' : '≈ '
  const per100 = food.nutrients
  const portion = recipe.portion
  const portionGrams = recipe.portions === undefined ? undefined : recipe.weight / recipe.portions
  const grams = (n: number) => t.unit.g(f.number(n))

  // takeNew saves the recipe again, its ingredients taking their foods' values
  // now; a retry from the status does as much.
  function takeNew() {
    again.mutate(
      { body: recipeInput(food) },
      {
        onSuccess: (answer) => {
          const p = answer.food.recipe?.portion
          sayChange(
            {
              title: t.recipes.savedPer(
                answer.food.name,
                t.unit.kcal(f.number(p?.kcal ?? answer.food.nutrients.kcal)),
                p ? t.recipes.perPortion : t.recipes.per100g,
              ),
              lines: answer.warnings.map((w) => warningOf(w, t)),
            },
            { last: { action: 'recipe.save', entity: answer.food.id }, date: 'today' },
          )
        },
      },
    )
  }

  return (
    <article aria-labelledby={`food-${food.id}`} className="space-y-4">
      <header className="flex items-start gap-3">
        <FoodAvatar name={food.name} large />
        <div className="flex min-h-14 min-w-0 flex-1 flex-col justify-center">
          <h1 id={`food-${food.id}`} className="text-title break-words">
            {food.name}
          </h1>
          <p className="mt-1 flex flex-wrap items-center gap-1.5 text-small text-muted-foreground">
            <Badge tone="primary">{t.recipes.mark}</Badge>
            {food.archived && <Badge>{t.foods.archivedMark}</Badge>}
            <span>
              {t.recipes.ingredients(items.length)}
              {recipe.portions !== undefined &&
                ` · ${t.recipes.portions(f.number(recipe.portions), recipe.portions)}`}
            </span>
          </p>
        </div>
      </header>
      {food.archived && (
        <p className="rounded-md bg-surface-2 px-3 py-2 text-small">{t.recipes.archivedNote}</p>
      )}
      <section aria-label={t.recipes.energy} className={cn(card, 'space-y-4')}>
        <p className="flex flex-wrap items-baseline gap-x-2">
          <span className="text-display">
            {portion ? f.number(portion.kcal) : `${approx}${f.number(per100.kcal)}`}
          </span>
          <span className="text-muted-foreground">
            {portion ? t.recipes.kcalPerPortion : t.recipes.kcalPer100}
          </span>
        </p>
        <EnergySplit nutrients={portion ?? per100} />
        <dl className="grid grid-cols-3 gap-2 border-t pt-3">
          {portionGrams !== undefined && (
            <div className="min-w-0">
              <dt className="text-caption text-muted-foreground">{t.recipes.portion}</dt>
              <dd className="text-small font-semibold">
                {approx}
                {grams(portionGrams)}
              </dd>
            </div>
          )}
          <div className="min-w-0">
            <dt className="text-caption text-muted-foreground">{t.recipes.per100}</dt>
            <dd className="text-small font-semibold">
              {approx}
              {t.unit.kcal(f.number(per100.kcal))}
            </dd>
          </div>
          <div className="min-w-0">
            <dt className="text-caption text-muted-foreground">{t.recipes.whole}</dt>
            <dd className="text-small font-semibold">{t.unit.kcal(f.number(recipe.total.kcal))}</dd>
          </div>
          {portionGrams === undefined && (
            <div className="min-w-0">
              <dt className="text-caption text-muted-foreground">{t.recipes.weight}</dt>
              <dd className="text-small font-semibold">
                {approx}
                {grams(recipe.weight)}
              </dd>
            </div>
          )}
        </dl>
        <div className="flex flex-wrap items-start gap-x-3 gap-y-2 border-t pt-3">
          <Scale aria-hidden className="mt-0.5 size-4.5 shrink-0 text-muted-foreground" />
          <div className="min-w-0 flex-1 basis-48 space-y-0.5">
            <p className="font-medium">
              {weighed ? t.recipes.weighed(grams(recipe.weight)) : t.recipes.notWeighed}
            </p>
            <p className="text-small text-muted-foreground">
              {weighed
                ? [
                    ...(recipe.portions === undefined || portionGrams === undefined
                      ? []
                      : [
                          t.recipes.portionsOf(
                            t.recipes.portions(f.number(recipe.portions), recipe.portions),
                            grams(portionGrams),
                          ),
                        ]),
                    ...(recipe.raw_weight === undefined
                      ? []
                      : [t.recipes.ofRaw(f.percent((recipe.weight / recipe.raw_weight) * 100, 0))]),
                  ].join(' · ')
                : t.recipes.notWeighedWhy(grams(recipe.weight))}
            </p>
          </div>
          {!weighed && !food.archived && (
            <Button
              ref={weighButton}
              variant="outline"
              size="sm"
              onClick={() => {
                setSheet('weigh')
              }}
            >
              {t.recipes.weighIt}
            </Button>
          )}
        </div>
      </section>
      <div className="grid grid-cols-2 gap-2">
        {!food.archived && <LogFood food={eaten} className="col-span-2" />}
        {!food.archived && (
          <Button
            ref={cookedButton}
            variant="outline"
            onClick={() => {
              setSheet('cooked')
            }}
          >
            <CookingPot aria-hidden />
            {t.recipes.cookedAgain}
          </Button>
        )}
        <Link
          to="/foods/$id"
          params={{ id: food.id }}
          search={{ ...search, edit: true }}
          state={{ fromFood: true }}
          className={buttonVariants({ variant: 'outline' })}
        >
          <Pencil aria-hidden />
          {t.foods.edit}
        </Link>
        {food.archived && <ArchiveButton food={food} archive={archive} />}
      </div>
      <SaveStatus change={archive} />
      {changed.length > 0 && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md bg-surface-2 px-3 py-2.5 text-small">
          <Info aria-hidden className="size-4 shrink-0 text-muted-foreground" />
          <p className="min-w-0 flex-1 basis-48">
            {t.recipes.changed(f.list(changed.map((i) => i.name)))}
          </p>
          <Button variant="outline" size="sm" disabled={again.isPending} onClick={takeNew}>
            <RefreshCw aria-hidden />
            {t.recipes.takeNew}
          </Button>
          <SaveStatus change={again} onRetry={takeNew} />
        </div>
      )}
      <section aria-labelledby={`${id}-items`} className={cn(card, 'space-y-2')}>
        <div className="flex items-baseline justify-between gap-2">
          <h2 id={`${id}-items`} className="text-h3">
            {t.recipes.ingredientsTitle}
          </h2>
          <span className="text-small text-muted-foreground">{f.number(items.length)}</span>
        </div>
        <ul aria-labelledby={`${id}-items`} className="divide-y">
          {items.map((item, i) => {
            const words = (
              <>
                <span className="min-w-0 flex-1 space-y-0.5">
                  <span className="flex flex-wrap items-center gap-1.5">
                    <span className="font-medium break-words">{item.name}</span>
                    {item.brand && <span className="text-muted-foreground">· {item.brand}</span>}
                    {item.source === 'recipe' && <Badge tone="primary">{t.recipes.mark}</Badge>}
                  </span>
                  <span className="block text-small text-muted-foreground">
                    {ingredientAmount(item, t, f)}
                    {item.changed === true && ` · ${t.recipes.changedMark}`}
                  </span>
                </span>
                <span className="shrink-0 text-sm whitespace-nowrap text-muted-foreground">
                  {t.unit.kcal(f.number(item.nutrients.kcal))}
                </span>
              </>
            )
            return (
              <li key={`${item.food_id}-${String(i)}`}>
                {item.source === 'recipe' ? (
                  <Link
                    to="/foods/$id"
                    params={{ id: item.food_id }}
                    search={{ q: search.q, filter: search.filter, order: search.order }}
                    aria-label={t.recipes.open(item.name)}
                    className="-mx-2 flex min-h-11 items-center gap-3 rounded-md px-2 py-2.5 outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {words}
                    <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
                  </Link>
                ) : (
                  <span className="flex items-center gap-3 py-2.5">{words}</span>
                )}
              </li>
            )
          })}
        </ul>
        <p className="flex justify-between gap-2 border-t pt-2.5 text-small font-semibold">
          <span>{t.recipes.rawSum}</span>
          <span>
            {recipe.raw_weight === undefined ? '' : `${grams(recipe.raw_weight)} · `}
            {t.unit.kcal(f.number(recipe.total.kcal))}
          </span>
        </p>
        <p className="text-caption text-muted-foreground">{t.recipes.valuesOfSave}</p>
      </section>
      <NutritionTable food={food} />
      {food.note && (
        <section className={cn(card, 'space-y-1.5')}>
          <h2 className="text-h3">{t.foods.note}</h2>
          <p className="whitespace-pre-line text-muted-foreground">{food.note}</p>
        </section>
      )}
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 px-1 text-small">
        <dt className="text-muted-foreground">{t.foods.from}</dt>
        <dd>{t.foods.sources.recipe}</dd>
        {!use.isError && (
          <>
            <dt className="text-muted-foreground">{t.foods.logged}</dt>
            <dd>
              {use.data === undefined ? (
                <span className="inline-block h-3 w-24 animate-pulse rounded-sm bg-surface-2 motion-reduce:animate-none">
                  <span className="sr-only">{t.loading}</span>
                </span>
              ) : use.data.last_use === undefined ? (
                t.foods.notLogged
              ) : (
                t.foods.timesLast(
                  f.number(use.data.meals),
                  f.calendarDay(use.data.last_use.day, timeZone),
                )
              )}
            </dd>
          </>
        )}
      </dl>
      {!food.archived && (
        <div>
          <ArchiveButton food={food} archive={archive} small />
        </div>
      )}
      <CookedAgain
        food={food}
        weigh={sheet === 'weigh'}
        open={sheet !== undefined}
        onOpenChange={(open) => {
          if (!open) {
            setSheet(undefined)
          }
        }}
        opener={sheet === 'weigh' ? weighButton : cookedButton}
      />
    </article>
  )
}

// ArchiveButton archives a recipe's food, or brings it back, which a
// toast says, with Undo.
function ArchiveButton({
  food,
  archive,
  small = false,
}: {
  food: Food
  archive: ReturnType<typeof useSaveFood>
  small?: boolean
}) {
  const t = useMessages()

  return (
    <Button
      variant="outline"
      size={small ? 'sm' : 'default'}
      disabled={archive.isPending}
      onClick={() => {
        archive.mutate(
          { body: { ...fieldsOf(food), archived: !food.archived } },
          {
            onSuccess: (saved) => {
              sayChange(
                {
                  title: saved.archived ? t.toast.archived(saved.name) : t.toast.inUse(saved.name),
                },
                { last: { action: 'food.save', entity: saved.id }, date: 'today' },
              )
            },
          },
        )
      }}
    >
      {food.archived ? <ArchiveRestore aria-hidden /> : <Archive aria-hidden />}
      {food.archived ? t.foods.unarchive : t.recipes.archive}
    </Button>
  )
}
