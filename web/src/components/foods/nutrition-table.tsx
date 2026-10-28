import { useId, useState } from 'react'

import type { Food } from '@/api/types'
import { SegmentedControl } from '@/components/ui/segmented'
import type { Messages } from '@/i18n/en'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// The rows of the table, in a label's order: what is "of which" another
// is set in under it.
const rows = [
  { key: 'kcal', within: false },
  { key: 'protein', within: false },
  { key: 'carbs', within: false },
  { key: 'sugars', within: true },
  { key: 'polyols', within: true },
  { key: 'fiber', within: false },
  { key: 'fat', within: false },
  { key: 'saturated_fat', within: true },
  { key: 'salt', within: false },
  { key: 'alcohol', within: false },
] as const

// A View is an amount the values are shown for: its words, and its
// share of the amount the food's values are for.
interface View {
  label: string
  factor: number
}

// viewsOf are the amounts a food's values can be shown for: what they
// are for (100 g or ml, or a portion), and each of its servings; of a
// portion of known grams, 100 g too. A portion's servings, in grams, need
// its grams.
function viewsOf(food: Food, t: Messages, number: (n: number) => string): View[] {
  const unit = food.basis === '100ml' ? 'ml' : 'g'
  const servings = food.servings ?? []
  if (food.basis !== 'portion') {
    return [
      { label: t.foods.hundred(unit), factor: 1 },
      ...servings.map((s) => ({
        label: t.foods.servingOf(s.name, t.unit[unit](number(s.amount))),
        factor: s.amount / 100,
      })),
    ]
  }
  const grams = food.portion_grams
  if (grams === undefined || grams <= 0) {
    return [{ label: t.foods.onePortion, factor: 1 }]
  }
  return [
    { label: t.foods.onePortion, factor: 1 },
    { label: t.foods.hundred('g'), factor: 100 / grams },
    ...servings.map((s) => ({
      label: t.foods.servingOf(s.name, t.unit.g(number(s.amount))),
      factor: s.amount / grams,
    })),
  ]
}

// NutritionTable is what a food brings, as a label lists it, for 100 g
// (or ml, or a portion) or for one of its servings, which the user
// picks: segments while they fit, else a list.
export function NutritionTable({ food }: { food: Food }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const views = viewsOf(food, t, (n) => f.number(n, 1))
  const [chosen, setChosen] = useState(0)
  const view = views[chosen] ?? views[0]
  const factor = view?.factor ?? 1
  const n = food.nutrients

  function value(key: (typeof rows)[number]['key'], v: number): string {
    const scaled = v * factor
    if (key === 'kcal') {
      return t.unit.kcal(f.number(scaled))
    }
    // Salt is weighed in hundredths on a label.
    return t.unit.g(key === 'salt' ? f.number(scaled, 2) : f.grams(scaled))
  }

  return (
    <section
      aria-labelledby={`${id}-title`}
      className="space-y-3 rounded-lg bg-card p-4 shadow-card lg:p-5"
    >
      <h2 id={`${id}-title`} className="text-h3">
        {t.foods.nutrition}
      </h2>
      {views.length > 1 && views.length <= 3 && (
        <SegmentedControl
          legend={t.foods.valuesFor}
          hideLegend
          options={views.map((v, i) => ({ value: String(i), label: v.label }))}
          value={String(chosen)}
          onChange={(v) => {
            setChosen(Number(v))
          }}
        />
      )}
      {views.length > 3 && (
        <label className="flex items-center gap-2">
          <span className={cn(fieldLabel, 'shrink-0')}>{t.foods.valuesFor}</span>
          <select
            value={chosen}
            className={inputClass}
            onChange={(e) => {
              setChosen(Number(e.currentTarget.value))
            }}
          >
            {views.map((v, i) => (
              <option key={v.label} value={i}>
                {v.label}
              </option>
            ))}
          </select>
        </label>
      )}
      <table aria-labelledby={`${id}-title`} className="w-full text-body">
        {views.length === 1 && view && (
          <caption className={cn(fieldLabel, 'pb-1 text-left')}>
            {t.foods.valuesFor} {view.label}
          </caption>
        )}
        <tbody className="divide-y">
          {rows.map(({ key, within }) => {
            const v = n[key]
            if (v === undefined) {
              return null
            }
            return (
              <tr key={key}>
                <th
                  scope="row"
                  className={cn(
                    'py-2 text-left',
                    within ? 'pl-4 font-normal text-muted-foreground' : 'font-medium',
                  )}
                >
                  {t.foods.row[key]}
                </th>
                <td className={cn('py-2 text-right', key === 'kcal' && 'font-semibold')}>
                  {value(key, v)}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </section>
  )
}
