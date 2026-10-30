import { useId, type ReactNode } from 'react'

import { AmountField } from '@/components/day/amount-field'
import {
  entryAmount,
  entryValues,
  inUnit,
  sayWrong,
  unitsOf,
  type FoodEntry,
  type Wrong,
} from '@/components/day/meal-entries'
import { entryGrams, entryUnitLabel, isRecipe } from '@/components/recipes/recipe-values'
import { Badge } from '@/components/ui/badge'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// IngredientRow is an ingredient of a recipe being written: its food's
// name, what it brings (and its grams, given in another unit), and its
// amount with its unit, a menu of the food's units when it has more,
// which works the amount out anew. was says how much it was, when
// changed; what is wrong with the amount is said under it; children are
// the row's other controls (its menu).
export function IngredientRow({
  entry,
  wrong,
  was,
  onChange,
  children,
}: {
  entry: FoodEntry
  wrong: Wrong | undefined
  was?: string | undefined
  onChange: (entry: FoodEntry) => void
  children?: ReactNode
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const food = entry.food
  const name = food.brand ? `${food.name} · ${food.brand}` : food.name
  const n = entryAmount(entry, f.parse)
  const values = entryValues(entry, f.parse)
  const grams = entryGrams(entry, f.parse)
  const units = unitsOf(food)
  const given = entry.unit === 'g' || entry.unit === 'ml'

  return (
    <li className="space-y-1 py-2">
      <div className="flex items-center gap-2">
        <span className="min-w-0 flex-1 space-y-0.5">
          <span className="flex flex-wrap items-center gap-1.5 text-sm">
            <span className="break-words">{name}</span>
            {isRecipe(food) && <Badge tone="primary">{t.recipes.mark}</Badge>}
          </span>
          <span className="block text-caption text-muted-foreground">
            {values === undefined || n === undefined
              ? '—'
              : [
                  t.unit.kcal(f.number(values.kcal)),
                  ...(given || grams === undefined ? [] : [t.unit.g(f.number(grams))]),
                  ...(was === undefined ? [] : [t.recipes.was(was)]),
                ].join(' · ')}
          </span>
        </span>
        <AmountField
          value={entry.amount}
          // A recipe's share is a fraction, whose / a keyboard of digits lacks.
          inputMode={isRecipe(food) ? 'text' : 'decimal'}
          aria-label={t.meal.amount(name)}
          aria-invalid={wrong !== undefined}
          aria-describedby={wrong ? `${id}-wrong` : undefined}
          onChange={(e) => {
            // Typed: what a unit chosen worked out is no more.
            const { exact: _typed, ...rest } = entry
            onChange({ ...rest, amount: e.currentTarget.value })
          }}
          unit={units.length > 1 ? undefined : entryUnitLabel(food, entry.unit, n ?? 1, t)}
          select={
            units.length > 1
              ? {
                  'aria-label': t.log.unit(name),
                  value: entry.unit,
                  onChange: (e) => {
                    onChange(inUnit(entry, e.currentTarget.value, f.parse, f.field))
                  },
                  children: units.map((u) => (
                    <option key={u} value={u}>
                      {entryUnitLabel(food, u, n ?? Number.NaN, t)}
                    </option>
                  )),
                }
              : undefined
          }
        />
        {children}
      </div>
      {wrong && (
        <p id={`${id}-wrong`} className="text-sm text-destructive">
          {sayWrong(wrong, t, f.number)}
        </p>
      )}
    </li>
  )
}
