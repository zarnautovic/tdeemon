import type { Dish } from '@/components/recipes/recipe-values'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// DishSummary is what a recipe being written comes to, as it is typed: a
// portion's kcal and macros, or else 100 g's, and the whole dish's kcal
// and grams; ≈ on grams, and what is per gram, while the dish is not
// weighed, never on a portion. before is the kcal it had before (of a
// portion, or 100 g), said beside.
export function DishSummary({ dish, before }: { dish: Dish; before?: number | undefined }) {
  const t = useMessages()
  const f = useFormat()
  const approx = dish.weighed ? '' : '≈ '
  const main = dish.portion ?? dish.per100

  return (
    <div aria-live="polite" className="space-y-0.5">
      <p className="flex flex-wrap items-baseline gap-x-2">
        <span className="text-small text-muted-foreground">
          {dish.portion ? t.recipes.perPortionLabel : t.recipes.per100Label}
        </span>
        <span className="font-semibold">
          {main === undefined
            ? '—'
            : `${dish.portion ? '' : approx}${t.unit.kcal(f.number(main.kcal))}`}
        </span>
        {main !== undefined && (
          <span className="text-small text-muted-foreground">
            {[
              ...(before === undefined ? [] : [t.recipes.was(f.number(before))]),
              `${t.day.short.protein} ${f.grams(main.protein)}`,
              `${t.day.short.carbs} ${f.grams(main.carbs)}`,
              `${t.day.short.fat} ${f.grams(main.fat)}`,
            ].join(' · ')}
          </span>
        )}
      </p>
      <p className="text-small text-muted-foreground">
        {t.recipes.wholeLine(
          t.unit.kcal(f.number(dish.total.kcal)),
          dish.weight === undefined ? '—' : `${approx}${t.unit.g(f.number(dish.weight))}`,
        )}
        {dish.portion &&
          dish.per100 &&
          ` · ${t.recipes.per100Label} ${approx}${t.unit.kcal(f.number(dish.per100.kcal))}`}
      </p>
    </div>
  )
}
