import { X } from 'lucide-react'
import { useEffect, useId, useRef, useState, type SubmitEvent } from 'react'

import { detailOf, statusOf } from '@/api/errors'
import { useCreateFood, useSaveFood } from '@/api/foods'
import type { components } from '@/api/schema'
import type { Food } from '@/api/types'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'
import { uuidv7 } from '@/lib/uuid'

type Upsert = components['schemas']['UpsertFoodInputBody']
type Nutrients = components['schemas']['FoodNutrientsBody']

// The most a serving may be, in the food's unit (servingBody).
const maxServing = 10000

// The nutrients a food must have, and those it may.
const needed = ['kcal', 'protein', 'carbs', 'fat'] as const
const optional = ['fiber', 'saturated_fat', 'sugars', 'salt', 'polyols', 'alcohol'] as const
type Nutrient = (typeof needed)[number] | (typeof optional)[number]

const bases = ['100g', '100ml', 'portion'] as const
const sources = ['label', 'estimate', 'user', 'dish'] as const

// A row of a list the form edits (a serving, another name), with a key of
// its own, as rows come and go.
interface Row<T> {
  key: string
  value: T
}

// Draft is the food as typed: numbers as text, in the language.
interface Draft {
  name: string
  brand: string
  basis: Food['basis']
  nutrients: Record<Nutrient, string>
  portionGrams: string
  servings: Row<{ name: string; amount: string }>[]
  aliases: Row<string>[]
  source: Food['source']
  note: string
}

function row<T>(value: T): Row<T> {
  return { key: uuidv7(), value }
}

// FoodForm makes a food, or corrects one: its name and brand, what its
// values are for and the values, a portion's grams, its servings and
// other names, where the values come from, and a note. A save replaces
// the whole food. onSaved gives the food as stored.
export function FoodForm({ food, onSaved }: { food?: Food; onSaved?: (food: Food) => void }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  // A food the user has is saved as corrected; a new one is made, and
  // refused when a food of its name and brand is in the way.
  const upsert = useSaveFood()
  const create = useCreateFood()
  const save = food ? upsert : create
  const form = useRef<HTMLFormElement>(null)
  // Every decimal a value has, near enough: a save leaves untouched
  // values as they are.
  const field = (n: number | undefined) => (n === undefined ? '' : f.field(n, 6))
  const [draft, setDraft] = useState<Draft>(() => ({
    name: food?.name ?? '',
    brand: food?.brand ?? '',
    basis: food?.basis ?? '100g',
    nutrients: {
      kcal: field(food?.nutrients.kcal),
      protein: field(food?.nutrients.protein),
      carbs: field(food?.nutrients.carbs),
      fat: field(food?.nutrients.fat),
      fiber: field(food?.nutrients.fiber),
      saturated_fat: field(food?.nutrients.saturated_fat),
      sugars: field(food?.nutrients.sugars),
      salt: field(food?.nutrients.salt),
      polyols: field(food?.nutrients.polyols),
      alcohol: field(food?.nutrients.alcohol),
    },
    portionGrams: field(food?.portion_grams),
    servings: (food?.servings ?? []).map((s) => row({ name: s.name, amount: field(s.amount) })),
    aliases: (food?.aliases ?? []).map((a) => row(a)),
    source: food?.source ?? 'label',
    note: food?.note ?? '',
  }))
  // The fields that are not what a food can have: needed and empty, or
  // not a number.
  const [wrong, setWrong] = useState<ReadonlyMap<string, string>>(new Map())
  const unit = draft.basis === '100ml' ? 'ml' : 'g'
  const set = (change: Partial<Draft>) => {
    setDraft((d) => ({ ...d, ...change }))
  }

  // body is the food the draft says, or the fields that are wrong.
  function body(): { body: Upsert } | { wrong: ReadonlyMap<string, string> } {
    const errors = new Map<string, string>()
    const number = (key: string, typed: string, isNeeded: boolean): number | undefined => {
      if (typed.trim() === '') {
        if (isNeeded) {
          errors.set(key, t.foods.needed)
        }
        return undefined
      }
      const n = f.parse(typed)
      // A number below 0 is a number all the same, of which no food has
      // less than none.
      const sign = /^\s*[-\u2212]/.exec(typed)
      if (sign && f.parse(typed.slice(sign[0].length)) !== undefined) {
        errors.set(key, t.foods.negative)
      } else if (n === undefined) {
        errors.set(key, t.day.notANumber)
      }
      return n
    }
    const name = draft.name.trim()
    if (name === '') {
      errors.set('name', t.foods.needed)
    }
    const nutrients: Nutrients = {
      kcal: number('kcal', draft.nutrients.kcal, true) ?? 0,
      protein: number('protein', draft.nutrients.protein, true) ?? 0,
      carbs: number('carbs', draft.nutrients.carbs, true) ?? 0,
      fat: number('fat', draft.nutrients.fat, true) ?? 0,
    }
    for (const k of optional) {
      const n = number(k, draft.nutrients[k], false)
      if (n !== undefined) {
        nutrients[k] = n
      }
    }
    const portionGrams =
      draft.basis === 'portion' ? number('portionGrams', draft.portionGrams, false) : undefined
    const servings = draft.servings.flatMap(({ key, value }) => {
      if (value.name.trim() === '' && value.amount.trim() === '') {
        return []
      }
      let amount = number(`serving ${key}`, value.amount, true)
      // As the server bounds a serving.
      if (amount !== undefined && (amount <= 0 || amount > maxServing)) {
        errors.set(`serving ${key}`, t.foods.servingBounds)
        amount = undefined
      }
      if (value.name.trim() === '') {
        errors.set(`serving name ${key}`, t.foods.needed)
      }
      return amount === undefined ? [] : [{ name: value.name.trim(), amount }]
    })
    const aliases = draft.aliases.map((a) => a.value.trim()).filter((a) => a !== '')
    if (errors.size > 0) {
      return { wrong: errors }
    }
    const brand = draft.brand.trim()
    const note = draft.note.trim()
    const fields: Upsert = {
      ...(food ? { id: food.id, archived: food.archived } : {}),
      name,
      ...(brand ? { brand } : {}),
      basis: draft.basis,
      nutrients,
      ...(portionGrams === undefined ? {} : { portion_grams: portionGrams }),
      ...(servings.length > 0 ? { servings } : {}),
      ...(aliases.length > 0 ? { aliases } : {}),
      source: draft.source,
      ...(note ? { note } : {}),
    }
    return { body: fields }
  }

  // send saves the draft, or says what in it is wrong.
  function send() {
    const made = body()
    if ('wrong' in made) {
      setWrong(made.wrong)
      return
    }
    setWrong(new Map())
    const done = { onSuccess: (saved: Food) => onSaved?.(saved) }
    if (food) {
      upsert.mutate({ body: made.body }, done)
    } else {
      create.mutate({ body: made.body }, done)
    }
  }

  // A draft that cannot be sent takes the focus to its first wrong field.
  useEffect(() => {
    if (wrong.size > 0) {
      form.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus()
    }
  }, [wrong])

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    send()
  }

  // A text field of the form, with its label and what is wrong with it: a
  // number's (decimal), or words of at most maxLength.
  function text(
    key: string,
    label: string,
    value: string,
    onChange: (v: string) => void,
    kind: { decimal: true } | { maxLength: number },
  ) {
    const error = wrong.get(key)
    return (
      <div className="space-y-1">
        <label htmlFor={`${id}-${key}`} className={cn(fieldLabel, 'block')}>
          {label}
        </label>
        <input
          id={`${id}-${key}`}
          value={value}
          autoComplete="off"
          {...('decimal' in kind
            ? { inputMode: 'decimal' as const }
            : { maxLength: kind.maxLength })}
          aria-invalid={error !== undefined}
          aria-describedby={error === undefined ? undefined : `${id}-${key}-error`}
          className={inputClass}
          onChange={(e) => {
            onChange(e.currentTarget.value)
          }}
        />
        {error && (
          <p id={`${id}-${key}-error`} className="text-xs text-destructive">
            {error}
          </p>
        )}
      </div>
    )
  }

  const status = statusOf(save.error)
  let refused
  if (save.isError && status === 409) {
    refused = t.foods.conflict(detailOf(save.error))
  } else if (save.isError && status === 422) {
    refused = t.foods.refused(detailOf(save.error))
  }

  return (
    <form ref={form} onSubmit={submit} noValidate className="space-y-4">
      {text(
        'name',
        t.foods.name,
        draft.name,
        (name) => {
          set({ name })
        },
        { maxLength: 200 },
      )}
      {text(
        'brand',
        t.foods.brand,
        draft.brand,
        (brand) => {
          set({ brand })
        },
        { maxLength: 100 },
      )}
      <div className="space-y-1">
        <label htmlFor={`${id}-basis`} className={cn(fieldLabel, 'block')}>
          {t.foods.basis}
        </label>
        <select
          id={`${id}-basis`}
          value={draft.basis}
          className={inputClass}
          onChange={(e) => {
            const chosen = bases.find((b) => b === e.currentTarget.value)
            if (chosen) {
              set({ basis: chosen })
            }
          }}
        >
          {bases.map((b) => (
            <option key={b} value={b}>
              {t.foods.per[b]}
            </option>
          ))}
        </select>
      </div>
      <div className="grid grid-cols-2 gap-3">
        {needed.map((k) => (
          <div key={k}>
            {text(
              k,
              t.foods.nutrient[k],
              draft.nutrients[k],
              (v) => {
                set({ nutrients: { ...draft.nutrients, [k]: v } })
              },
              { decimal: true },
            )}
          </div>
        ))}
      </div>
      <details className="rounded-lg border p-3" open={optional.some((k) => wrong.has(k))}>
        <summary className="cursor-pointer text-sm">{t.foods.moreNutrients}</summary>
        <div className="mt-3 grid grid-cols-2 gap-3">
          {optional.map((k) => (
            <div key={k}>
              {text(
                k,
                t.foods.nutrient[k],
                draft.nutrients[k],
                (v) => {
                  set({ nutrients: { ...draft.nutrients, [k]: v } })
                },
                { decimal: true },
              )}
            </div>
          ))}
        </div>
      </details>
      {draft.basis === 'portion' &&
        text(
          'portionGrams',
          t.foods.portionGrams,
          draft.portionGrams,
          (portionGrams) => {
            set({ portionGrams })
          },
          { decimal: true },
        )}
      <fieldset className="space-y-2">
        <legend className={fieldLabel}>{t.foods.servings}</legend>
        {draft.servings.map(({ key, value }, i) => (
          <div key={key} className="flex items-end gap-2">
            <div className="flex-1">
              {text(
                `serving name ${key}`,
                t.foods.serving(i + 1),
                value.name,
                (name) => {
                  set({
                    servings: draft.servings.map((r) =>
                      r.key === key ? { key, value: { ...r.value, name } } : r,
                    ),
                  })
                },
                { maxLength: 50 },
              )}
            </div>
            <div className="w-28">
              {text(
                `serving ${key}`,
                t.foods.servingAmount(unit, i + 1),
                value.amount,
                (amount) => {
                  set({
                    servings: draft.servings.map((r) =>
                      r.key === key ? { key, value: { ...r.value, amount } } : r,
                    ),
                  })
                },
                { decimal: true },
              )}
            </div>
            <Button
              variant="ghost"
              size="icon-lg"
              aria-label={t.foods.removeServing(value.name)}
              onClick={() => {
                set({ servings: draft.servings.filter((r) => r.key !== key) })
              }}
            >
              <X aria-hidden />
            </Button>
          </div>
        ))}
        <Button
          variant="outline"
          onClick={() => {
            set({ servings: [...draft.servings, row({ name: '', amount: '' })] })
          }}
        >
          {t.foods.addServing}
        </Button>
      </fieldset>
      <fieldset className="space-y-2">
        <legend className={fieldLabel}>{t.foods.aliases}</legend>
        {draft.aliases.map(({ key, value }, i) => (
          <div key={key} className="flex items-end gap-2">
            <div className="flex-1">
              {text(
                `alias ${key}`,
                t.foods.alias(i + 1),
                value,
                (alias) => {
                  set({
                    aliases: draft.aliases.map((r) => (r.key === key ? { key, value: alias } : r)),
                  })
                },
                { maxLength: 100 },
              )}
            </div>
            <Button
              variant="ghost"
              size="icon-lg"
              aria-label={t.foods.removeAlias(value)}
              onClick={() => {
                set({ aliases: draft.aliases.filter((r) => r.key !== key) })
              }}
            >
              <X aria-hidden />
            </Button>
          </div>
        ))}
        <Button
          variant="outline"
          onClick={() => {
            set({ aliases: [...draft.aliases, row('')] })
          }}
        >
          {t.foods.addAlias}
        </Button>
      </fieldset>
      <div className="space-y-1">
        <label htmlFor={`${id}-source`} className={cn(fieldLabel, 'block')}>
          {t.foods.source}
        </label>
        <select
          id={`${id}-source`}
          value={draft.source}
          className={inputClass}
          onChange={(e) => {
            const chosen = sources.find((s) => s === e.currentTarget.value)
            if (chosen) {
              set({ source: chosen })
            }
          }}
        >
          {sources.map((s) => (
            <option key={s} value={s}>
              {t.foods.sources[s]}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-1">
        <label htmlFor={`${id}-note`} className={cn(fieldLabel, 'block')}>
          {t.foods.note}
        </label>
        <textarea
          id={`${id}-note`}
          value={draft.note}
          rows={2}
          maxLength={2000}
          className={cn(inputClass, 'h-auto py-2')}
          onChange={(e) => {
            set({ note: e.currentTarget.value })
          }}
        />
      </div>
      {refused ? (
        <p role="alert" className="text-sm text-destructive">
          {refused}
        </p>
      ) : food ? (
        <SaveStatus change={upsert} onRetry={send} />
      ) : (
        <SaveStatus change={create} onRetry={send} />
      )}
      <Button type="submit" size="lg" className="w-full" disabled={save.isPending}>
        {t.foods.save}
      </Button>
    </form>
  )
}
