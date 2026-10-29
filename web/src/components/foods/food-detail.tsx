import { useQuery } from '@tanstack/react-query'
import { Link, useSearch } from '@tanstack/react-router'
import { Archive, ArchiveRestore, Pencil } from 'lucide-react'

import { fieldsOf, useFoodUse, useSaveFood } from '@/api/foods'
import { sessionQuery } from '@/api/session'
import type { Food } from '@/api/types'
import { SaveStatus } from '@/components/day/save-status'
import { FoodAvatar } from '@/components/foods/food-avatar'
import { LogFood } from '@/components/foods/log-food'
import { NutritionTable } from '@/components/foods/nutrition-table'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

// A card of the page: 16 px within on a phone, 20 on a desktop.
const card = 'rounded-lg bg-card p-4 shadow-card lg:p-5'

// FoodDetail is a food of the user's, read: its name and brand, whether
// its values are an estimate, its energy and where the energy comes from,
// what it brings (NutritionTable), its servings, other names and note,
// where its values come from and how often it was logged. It logs the
// food today in the amount eaten last (LogFood), opens it to edit, and
// archives it or brings it back, which a toast says, with Undo.
export function FoodDetail({ food }: { food: Food }) {
  const t = useMessages()
  const f = useFormat()
  const search = useSearch({ from: '/app/foods/$id' })
  const use = useFoodUse(food.id)
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const archive = useSaveFood()
  const n = food.nutrients
  const unit = food.basis === '100ml' ? 'ml' : 'g'
  const last = use.data?.last_use
  // The food as a meal had it last, which the meal to log starts from.
  const eaten: Food = last === undefined ? food : { ...food, last_use: last }
  const servings = food.servings ?? []
  const aliases = food.aliases ?? []
  const barcodes = food.barcodes ?? []

  return (
    <article aria-labelledby={`food-${food.id}`} className="space-y-4">
      {/* A short name beside the middle of its mark, a long one from its top. */}
      <header className="flex items-start gap-3">
        <FoodAvatar name={food.name} large />
        <div className="flex min-h-14 min-w-0 flex-1 flex-col justify-center">
          <h1 id={`food-${food.id}`} className="text-title break-words">
            {food.name}
          </h1>
          {(Boolean(food.brand) || food.source === 'estimate' || food.archived) && (
            <p className="mt-1 flex flex-wrap items-center gap-1.5 text-small text-muted-foreground">
              {food.brand && <span className="mr-0.5">{food.brand}</span>}
              {food.source === 'estimate' && <Badge tone="warning">{t.meal.estimate}</Badge>}
              {food.archived && <Badge>{t.foods.archivedMark}</Badge>}
            </p>
          )}
        </div>
      </header>
      {food.archived && (
        <p className="rounded-md bg-surface-2 px-3 py-2 text-small">{t.foods.archivedNote}</p>
      )}
      <section aria-label={t.foods.energyFrom} className={cn(card, 'space-y-4')}>
        <p className="flex flex-wrap items-baseline gap-x-2">
          <span className="text-display">{f.number(n.kcal)}</span>
          <span className="text-muted-foreground">{t.foods.kcalPer(t.foods.per[food.basis])}</span>
        </p>
        <EnergySplit nutrients={n} />
      </section>
      <div className="grid grid-cols-2 gap-2 sm:flex sm:flex-wrap">
        {!food.archived && <LogFood food={eaten} className="col-span-2 sm:flex-1" />}
        <Link
          to="/foods/$id"
          params={{ id: food.id }}
          search={{ ...search, edit: true }}
          state={{ fromFood: true }}
          // Merged, as Button merges its own: the outline's border, not the base's.
          className={buttonVariants({ variant: 'outline' })}
        >
          <Pencil aria-hidden />
          {t.foods.edit}
        </Link>
        <Button
          variant="outline"
          disabled={archive.isPending}
          onClick={() => {
            archive.mutate(
              { body: { ...fieldsOf(food), archived: !food.archived } },
              {
                onSuccess: (saved) => {
                  sayChange(
                    {
                      title: saved.archived
                        ? t.toast.archived(saved.name)
                        : t.toast.inUse(saved.name),
                    },
                    // Of no day: after the changes of today's still on their way.
                    { last: { action: 'food.save', entity: saved.id }, date: 'today' },
                  )
                },
              },
            )
          }}
        >
          {food.archived ? <ArchiveRestore aria-hidden /> : <Archive aria-hidden />}
          {food.archived ? t.foods.unarchive : t.foods.archive}
        </Button>
      </div>
      <SaveStatus change={archive} />
      <NutritionTable food={food} />
      {(servings.length > 0 || aliases.length > 0 || barcodes.length > 0 || food.note) && (
        <section className={cn(card, 'space-y-4')}>
          {servings.length > 0 && (
            <div className="space-y-1.5">
              <h2 className="text-h3">{t.foods.servings}</h2>
              <ul className="divide-y">
                {servings.map((s) => (
                  <li key={s.name} className="flex items-baseline justify-between gap-3 py-2">
                    <span className="min-w-0 break-words">{s.name}</span>
                    <span className="shrink-0 text-muted-foreground">
                      {t.unit[unit](f.number(s.amount, 1))}
                      {food.basis !== 'portion' &&
                        ` · ${t.unit.kcal(f.number((n.kcal * s.amount) / 100))}`}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {aliases.length > 0 && (
            <div className="space-y-1.5">
              <h2 className="text-h3">{t.foods.aliases}</h2>
              <ul className="flex flex-wrap gap-1.5">
                {aliases.map((a) => (
                  <li key={a} className="rounded-sm bg-surface-2 px-2.5 py-1 text-small">
                    {a}
                  </li>
                ))}
              </ul>
            </div>
          )}
          {barcodes.length > 0 && (
            <div className="space-y-1.5">
              <h2 className="text-h3">{t.foods.barcodes}</h2>
              <ul className="flex flex-wrap gap-1.5">
                {barcodes.map((code) => (
                  <li
                    key={code}
                    className="rounded-sm bg-surface-2 px-2.5 py-1 text-small tabular-nums"
                  >
                    {code}
                  </li>
                ))}
              </ul>
            </div>
          )}
          {food.note && (
            <div className="space-y-1.5">
              <h2 className="text-h3">{t.foods.note}</h2>
              <p className="whitespace-pre-line text-muted-foreground">{food.note}</p>
            </div>
          )}
        </section>
      )}
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 px-1 text-small">
        <dt className="text-muted-foreground">{t.foods.from}</dt>
        <dd>{t.foods.sources[food.source]}</dd>
        {/* Not said when it did not come: the rest of the page holds. */}
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
    </article>
  )
}

// EnergySplit is where a food's energy comes from: a bar of the energy
// the macros bring, protein's and carbs' 4 kcal a gram and fat's 9, each
// in its colour, and each macro's grams and share in words.
function EnergySplit({ nutrients }: { nutrients: Food['nutrients'] }) {
  const t = useMessages()
  const f = useFormat()
  const parts = [
    { macro: 'protein', grams: nutrients.protein, kcal: nutrients.protein * 4, fill: 'bg-protein' },
    { macro: 'carbs', grams: nutrients.carbs, kcal: nutrients.carbs * 4, fill: 'bg-carbs' },
    { macro: 'fat', grams: nutrients.fat, kcal: nutrients.fat * 9, fill: 'bg-fat' },
  ] as const
  const sum = parts.reduce((s, p) => s + p.kcal, 0)

  return (
    <div className="space-y-3">
      <div aria-hidden className="flex h-2.5 gap-0.5 overflow-hidden rounded-full bg-surface-2">
        {sum > 0 &&
          parts.map((p) => (
            <span
              key={p.macro}
              className={p.fill}
              style={{ width: `${String((p.kcal / sum) * 100)}%` }}
            />
          ))}
      </div>
      <dl className="grid grid-cols-3 gap-2">
        {parts.map((p) => (
          <div key={p.macro} className="min-w-0">
            <dt className="flex items-center gap-1.5 text-caption text-muted-foreground">
              <span aria-hidden className={cn('size-2 shrink-0 rounded-full', p.fill)} />
              {t.day[p.macro]}
            </dt>
            <dd className="text-small font-semibold">
              {t.foods.share(f.grams(p.grams), f.percent(sum > 0 ? (p.kcal / sum) * 100 : 0, 0))}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
