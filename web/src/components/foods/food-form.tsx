import { Link } from '@tanstack/react-router'
import { Plus, ScanBarcode, TriangleAlert, X } from 'lucide-react'
import { useEffect, useId, useRef, useState, type ReactNode, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { conflictOf, detailOf, statusOf } from '@/api/errors'
import { useCreateFood, useSaveFood } from '@/api/foods'
import type { components } from '@/api/schema'
import type { Food } from '@/api/types'
import { amountWrong, sayWrong } from '@/components/day/meal-entries'
import { SaveStatus } from '@/components/day/save-status'
import { Scanner } from '@/components/day/scanner'
import { energyOff } from '@/components/foods/energy'
import { Button } from '@/components/ui/button'
import { SegmentedControl } from '@/components/ui/segmented'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { nameKey } from '@/lib/name-key'
import { fieldLabel, inputClass } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'
import { uuidv7 } from '@/lib/uuid'

type FoodFields = components['schemas']['FoodFields']

type Upsert = components['schemas']['UpsertFoodInputBody']
type Nutrients = components['schemas']['FoodNutrientsBody']

// The most a serving may be, in the food's unit (servingBody).
const maxServing = 10000

// The nutrients a food must have, and those it may.
const needed = ['kcal', 'protein', 'carbs', 'fat'] as const
const optional = ['fiber', 'saturated_fat', 'sugars', 'salt', 'polyols', 'alcohol'] as const
type Nutrient = (typeof needed)[number] | (typeof optional)[number]

// The nutrients whose grams add up, which 100 g or ml can hold so many of.
const mass = ['protein', 'carbs', 'fat', 'alcohol'] as const

const bases = ['100g', '100ml', 'portion'] as const
const sources = ['label', 'estimate', 'user', 'dish', 'off'] as const

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
  // The product's barcodes: scanned or typed (Scanner), each once.
  barcodes: string[]
  source: Food['source']
  note: string
}

function row<T>(value: T): Row<T> {
  return { key: uuidv7(), value }
}

// energyOfDraft is the energy typed, and what the values typed make, when
// the two are further apart than a label rounds (energyOff): of the four
// needed numbers, with the fibre, polyols and alcohol typed counted.
function energyOfDraft(
  typed: Record<Nutrient, string>,
  parse: (s: string) => number | undefined,
): { typed: number; values: number } | undefined {
  const kcal = parse(typed.kcal)
  const protein = parse(typed.protein)
  const carbs = parse(typed.carbs)
  const fat = parse(typed.fat)
  if (kcal === undefined || protein === undefined || carbs === undefined || fat === undefined) {
    return undefined
  }
  const n: Nutrients = { kcal, protein, carbs, fat }
  for (const k of ['fiber', 'polyols', 'alcohol'] as const) {
    const v = parse(typed[k])
    if (v !== undefined) {
      n[k] = v
    }
  }
  const values = energyOff(n)
  return values === undefined ? undefined : { typed: kcal, values }
}

// A compact form is of a food made in a meal being logged, its next step:
// what a food needs (its name, what its values are for, the four values)
// and the amount eaten now, in the unit of what the values are for, its
// other fields under More. It is sent by the button of its id, outside
// it, at the sheet's foot; onAdd gets the food saved and the amount.
interface Compact {
  id: string
  // The name to start with: what was searched for.
  name: string
  // The rest to start with: a product a barcode found, or the barcode
  // alone.
  start?: Partial<FoodFields>
  onAdd: (food: Food, amount: number) => void
}

// summaryClass is a More's summary: the whole row of its box to a
// finger, 44 px, its words where its box's padding has them.
const summaryClass = '-m-3 cursor-pointer p-3 text-sm'

// The fields a compact form shows outside More.
const outside = new Set<string>(['name', ...needed, 'mass', 'eaten'])

// unitOf is the unit of a food's amount, of what its values are for.
function unitOf(basis: Food['basis']): 'g' | 'ml' | 'portion' {
  return basis === '100ml' ? 'ml' : basis === 'portion' ? 'portion' : 'g'
}

// FoodForm makes a food, or corrects one: its name and brand, what its
// values are for and the values, a portion's grams, its servings and
// other names, where the values come from, and a note. A save replaces
// the whole food. onSaved gives the food as stored. A compact one makes
// a food in a meal being logged (Compact).
export function FoodForm({
  food,
  onSaved,
  compact,
}: {
  food?: Food
  onSaved?: (food: Food) => void
  compact?: Compact
}) {
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
  // What the form starts from: the food, or a compact one's start.
  const from: Partial<FoodFields> | undefined = food ?? compact?.start
  const [draft, setDraft] = useState<Draft>(() => ({
    name: from?.name ?? compact?.name ?? '',
    brand: from?.brand ?? '',
    basis: from?.basis ?? '100g',
    nutrients: {
      kcal: field(from?.nutrients?.kcal),
      protein: field(from?.nutrients?.protein),
      carbs: field(from?.nutrients?.carbs),
      fat: field(from?.nutrients?.fat),
      fiber: field(from?.nutrients?.fiber),
      saturated_fat: field(from?.nutrients?.saturated_fat),
      sugars: field(from?.nutrients?.sugars),
      salt: field(from?.nutrients?.salt),
      polyols: field(from?.nutrients?.polyols),
      alcohol: field(from?.nutrients?.alcohol),
    },
    portionGrams: field(from?.portion_grams),
    servings: (from?.servings ?? []).map((s) => row({ name: s.name, amount: field(s.amount) })),
    aliases: (from?.aliases ?? []).map((a) => row(a)),
    barcodes: from?.barcodes ?? [],
    source: from?.source ?? 'label',
    note: from?.note ?? '',
  }))
  // A compact form's amount eaten now, as typed; until it is, 100 g or ml,
  // or a portion.
  const [eatenTyped, setEaten] = useState<string | undefined>(undefined)
  const eaten = eatenTyped ?? (draft.basis === 'portion' ? f.field(1) : f.field(100))
  // The fields that are not what a food can have: needed and empty, or
  // not a number. A field changed since is not said wrong (edited) until
  // the next save, which tells it again; the others stay.
  const [wrong, setWrong] = useState<ReadonlyMap<string, string>>(new Map())
  const [edited, setEdited] = useState<ReadonlySet<string>>(new Set())
  const errorOf = (key: string) => (edited.has(key) ? undefined : wrong.get(key))
  const unit = draft.basis === '100ml' ? 'ml' : 'g'
  // The food in the way of the save, and the field it is in the way of,
  // while it holds what was sent: the name and brand, or the row of the
  // alias sent at the place the answer gives. A change of the form makes
  // it stale.
  const conflict = conflictOf(save.error)
  const sent = save.variables?.body
  let conflictKey: string | undefined
  if (conflict && sent && conflict.alias === undefined && conflict.barcode === undefined) {
    conflictKey =
      sent.name === draft.name.trim() && (sent.brand ?? '') === draft.brand.trim()
        ? 'name'
        : undefined
  } else if (conflict && sent && conflict.alias !== undefined) {
    const taken = sent.aliases?.[conflict.alias]
    const takenRow = draft.aliases.find((a) => taken !== undefined && a.value.trim() === taken)
    conflictKey = takenRow && `alias ${takenRow.key}`
  }
  const set = (change: Partial<Draft>) => {
    setDraft((d) => ({ ...d, ...change }))
    if (conflict) {
      save.reset()
    }
  }
  // Whether the camera is open for a barcode, or its digits typed.
  const [scanning, setScanning] = useState(false)
  // The field of a row just added (a serving's name, another name), which
  // takes the focus.
  const [added, setAdded] = useState<string | undefined>(undefined)
  useEffect(() => {
    if (added !== undefined) {
      document.getElementById(`${id}-${added}`)?.focus()
    }
  }, [added, id])
  // The energy the values typed make, when the energy typed is further
  // from it than a label rounds (energyOff): a warning under the macros,
  // with the way to take it, which does not keep the food from a save.
  const energy = energyOfDraft(draft.nutrients, f.parse)

  // tooMuch sets in errors what of nutrients the server refuses of a food
  // (limits.json, food.ValidatePer100): per 100 g or ml no more energy than
  // pure fat, no more grams than 100 can hold; a portion no more than a
  // feast, and of its grams known, as much per 100 g of it. A field said
  // wrong already stays so.
  function tooMuch(
    nutrients: Nutrients,
    portionGrams: number | undefined,
    errors: Map<string, string>,
  ) {
    const { maxKcalPer100, maxGramsPer100g, maxGramsPer100ml, maxKcalPerPortion } = limits.food
    const say = (key: string, message: string) => {
      if (!errors.has(key)) {
        errors.set(key, message)
      }
    }
    if (draft.basis === 'portion' && nutrients.kcal > maxKcalPerPortion) {
      say('kcal', t.foods.kcalPerPortion(f.number(maxKcalPerPortion)))
    }
    // The values per 100 g or ml: a portion's, scaled as the server does.
    let per100 = 1
    if (draft.basis === 'portion') {
      if (!portionGrams) {
        return
      }
      per100 = 100 / portionGrams
    }
    const in100 = draft.basis === '100ml' ? 'ml' : 'g'
    const most = draft.basis === '100ml' ? maxGramsPer100ml : maxGramsPer100g
    if (nutrients.kcal * per100 > maxKcalPer100) {
      say('kcal', t.foods.kcalPer100(f.number(maxKcalPer100), in100))
    }
    if (mass.reduce((sum, k) => sum + (nutrients[k] ?? 0) * per100, 0) > most) {
      say('mass', t.foods.massPer100(f.number(most), in100))
    }
    for (const k of optional) {
      const v = nutrients[k]
      if (v !== undefined && v * per100 > most) {
        say(k, t.foods.gramsPer100(f.number(most), in100))
      }
    }
  }

  // body is the food the draft says, with a compact form's amount eaten
  // now, or the fields that are wrong.
  function body(): { body: Upsert; eaten: number } | { wrong: ReadonlyMap<string, string> } {
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
    tooMuch(nutrients, portionGrams, errors)
    // The names of the servings, and the other names, as the server tells
    // them apart (food.Key): one given twice is refused. A food has so
    // many of each (limits.json): the first past them is said, whatever
    // else is wrong with it.
    const { maxServings, maxAliases } = limits.food
    const servingNames = new Set<string>()
    let servingsGiven = 0
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
      const named = value.name.trim()
      if (named === '') {
        errors.set(`serving name ${key}`, t.foods.needed)
      } else if (servingNames.has(nameKey(named))) {
        errors.set(`serving name ${key}`, t.foods.servingTwice)
      }
      servingNames.add(nameKey(named))
      servingsGiven += 1
      if (servingsGiven === maxServings + 1) {
        errors.set(
          `serving name ${key}`,
          t.foods.tooManyServings(f.number(maxServings), maxServings),
        )
      }
      return amount === undefined ? [] : [{ name: named, amount }]
    })
    const aliasNames = new Set<string>()
    let aliasesGiven = 0
    const aliases = draft.aliases.flatMap(({ key, value }) => {
      const alias = value.trim()
      if (alias === '') {
        return []
      }
      if (aliasNames.has(nameKey(alias))) {
        errors.set(`alias ${key}`, t.foods.aliasTwice)
      }
      aliasNames.add(nameKey(alias))
      aliasesGiven += 1
      if (aliasesGiven === maxAliases + 1) {
        errors.set(`alias ${key}`, t.foods.tooManyAliases(f.number(maxAliases), maxAliases))
      }
      return [alias]
    })
    // A compact form's amount, as an item may hold it (tooMuch).
    let amount = 0
    if (compact) {
      const n = number('eaten', eaten, true)
      const why = n === undefined ? undefined : amountWrong(undefined, unitOf(draft.basis), n)
      if (why !== undefined && !errors.has('eaten')) {
        errors.set('eaten', sayWrong(why, t, f.number))
      }
      amount = n ?? 0
    }
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
      ...(draft.barcodes.length > 0 ? { barcodes: draft.barcodes } : {}),
      source: draft.source,
      ...(note ? { note } : {}),
    }
    return { body: fields, eaten: amount }
  }

  // send saves the draft, or says what in it is wrong. A save on its way
  // is not sent again, whichever button or key sends it: a compact form's
  // is outside it.
  function send() {
    if (save.isPending) {
      return
    }
    const made = body()
    setEdited(new Set())
    if ('wrong' in made) {
      setWrong(made.wrong)
      return
    }
    setWrong(new Map())
    const done = {
      onSuccess: (saved: Food) => {
        sayChange(
          { title: t.toast.savedOf(saved.name) },
          // Of no day: after the changes of today's still on their way.
          { last: { action: 'food.save', entity: saved.id }, date: 'today' },
        )
        if (compact) {
          compact.onAdd(saved, made.eaten)
        } else {
          onSaved?.(saved)
        }
      },
    }
    if (food) {
      upsert.mutate({ body: made.body }, done)
    } else {
      create.mutate({ body: made.body }, done)
    }
  }

  // A draft that cannot be sent, or that another food is in the way of,
  // takes the focus to its first wrong field.
  useEffect(() => {
    if (wrong.size > 0 || conflictKey !== undefined) {
      form.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus()
    }
  }, [wrong, conflictKey])

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
    const error = errorOf(key)
    // The field another food is in the way of, which the refusal says;
    // one of the grams that add up to more than 100 can hold.
    const taken = error === undefined && key === conflictKey
    const weighed = mass.some((k) => k === key)
    const tooHeavy =
      error === undefined && errorOf('mass') !== undefined && weighed && value.trim() !== ''
    // The energy the warning under the macros is of.
    const warned = error === undefined && key === 'kcal' && energy !== undefined
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
          aria-invalid={error !== undefined || taken || tooHeavy}
          aria-describedby={
            error !== undefined
              ? `${id}-${key}-error`
              : taken
                ? `${id}-refused`
                : tooHeavy
                  ? `${id}-mass-error`
                  : warned
                    ? `${id}-energy`
                    : undefined
          }
          className={inputClass}
          onChange={(e) => {
            // Changed, the field is not said wrong; nor, of the grams that
            // add up, are they.
            const changed = weighed ? [key, 'mass'] : [key]
            if (changed.some((k) => wrong.has(k) && !edited.has(k))) {
              setEdited(new Set([...edited, ...changed]))
            }
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
  let refused: ReactNode
  if (conflict) {
    const by =
      conflict.alias === undefined ? (conflict.barcode === undefined ? 'name' : 'barcode') : 'alias'
    refused = <TakenBy foodId={conflict.foodId} by={by} />
  } else if (save.isError && status === 409) {
    refused = t.foods.conflict(detailOf(save.error))
  } else if (save.isError && status === 422) {
    refused = t.foods.refused(detailOf(save.error))
  }

  // The form's parts, laid out in full or compact.
  const nameField = text(
    'name',
    t.foods.name,
    draft.name,
    (name) => {
      set({ name })
    },
    { maxLength: 200 },
  )
  const brandField = text(
    'brand',
    t.foods.brand,
    draft.brand,
    (brand) => {
      set({ brand })
    },
    { maxLength: 100 },
  )
  const values = (
    <>
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
      {errorOf('mass') !== undefined && (
        <p id={`${id}-mass-error`} className="text-xs text-destructive">
          {errorOf('mass')}
        </p>
      )}
      <div aria-live="polite">
        {energy !== undefined && (
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md bg-warning/10 px-3 py-2.5 text-small text-warning">
            <TriangleAlert aria-hidden className="size-4 shrink-0" />
            <p id={`${id}-energy`} className="min-w-0 flex-1 basis-48">
              {t.foods.energyOff(f.number(energy.values), f.number(energy.typed))}
            </p>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                set({
                  nutrients: { ...draft.nutrients, kcal: f.field(Math.round(energy.values), 0) },
                })
              }}
            >
              {t.foods.takeKcal(f.number(energy.values))}
            </Button>
          </div>
        )}
      </div>
    </>
  )
  const otherValues = (
    <div className="grid grid-cols-2 gap-3">
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
  )
  const portionField =
    draft.basis === 'portion' &&
    text(
      'portionGrams',
      t.foods.portionGrams,
      draft.portionGrams,
      (portionGrams) => {
        set({ portionGrams })
      },
      { decimal: true },
    )
  const servingsField = (
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
            size="icon"
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
          const next = row({ name: '', amount: '' })
          set({ servings: [...draft.servings, next] })
          setAdded(`serving name ${next.key}`)
        }}
      >
        <Plus aria-hidden />
        {t.foods.addServing}
      </Button>
    </fieldset>
  )
  const aliasesField = (
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
            size="icon"
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
          const next = row('')
          set({ aliases: [...draft.aliases, next] })
          setAdded(`alias ${next.key}`)
        }}
      >
        <Plus aria-hidden />
        {t.foods.addAlias}
      </Button>
    </fieldset>
  )
  // The barcodes, in the full form: a compact one's come from its scan.
  const barcodesField = (
    <fieldset className="space-y-2">
      <legend className={fieldLabel}>{t.foods.barcodes}</legend>
      {draft.barcodes.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {draft.barcodes.map((code) => (
            <li
              key={code}
              className="flex items-center gap-0.5 rounded-full bg-surface-2 pl-3 text-small tabular-nums"
            >
              {code}
              <Button
                variant="ghost"
                size="icon-sm"
                className="rounded-full"
                aria-label={t.foods.removeBarcode(code)}
                onClick={() => {
                  set({ barcodes: draft.barcodes.filter((c) => c !== code) })
                }}
              >
                <X aria-hidden />
              </Button>
            </li>
          ))}
        </ul>
      )}
      {scanning ? (
        <div className="space-y-2 rounded-lg border p-3">
          <Scanner
            sendWith={t.foods.addCode}
            onCode={(code) => {
              setScanning(false)
              if (!draft.barcodes.includes(code)) {
                set({ barcodes: [...draft.barcodes, code] })
              }
            }}
          />
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setScanning(false)
            }}
          >
            {t.foods.stopScan}
          </Button>
        </div>
      ) : (
        <Button
          variant="outline"
          disabled={draft.barcodes.length >= limits.food.maxBarcodes}
          onClick={() => {
            setScanning(true)
          }}
        >
          <ScanBarcode aria-hidden />
          {t.foods.addBarcode}
        </Button>
      )}
    </fieldset>
  )
  const saveStatus = refused ? (
    <p id={`${id}-refused`} role="alert" className="text-sm text-destructive">
      {refused}
    </p>
  ) : food ? (
    <SaveStatus change={upsert} onRetry={send} />
  ) : (
    <SaveStatus change={create} onRetry={send} />
  )

  if (compact) {
    // More opens on a field of its said wrong, or another food's in the
    // way of.
    const moreWrong =
      [...wrong.keys()].some((k) => !outside.has(k)) ||
      (conflictKey !== undefined && !outside.has(conflictKey))
    return (
      <form id={compact.id} ref={form} onSubmit={submit} noValidate className="space-y-4">
        {nameField}
        <SegmentedControl
          legend={t.foods.basis}
          options={bases.map((b) => ({ value: b, label: t.foods.per[b] }))}
          value={draft.basis}
          onChange={(basis) => {
            set({ basis })
          }}
        />
        {values}
        <div className="grid grid-cols-2 gap-3">
          {text(
            'eaten',
            t.log.eatenNow[draft.basis],
            eaten,
            (typed) => {
              setEaten(typed)
            },
            { decimal: true },
          )}
        </div>
        <details className="rounded-lg border p-3" open={moreWrong}>
          <summary className={summaryClass}>{t.foods.moreFields}</summary>
          <div className="mt-3 space-y-4">
            {brandField}
            {otherValues}
            {portionField}
            {servingsField}
            {aliasesField}
          </div>
        </details>
        {saveStatus}
      </form>
    )
  }
  return (
    <form ref={form} onSubmit={submit} noValidate className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        {nameField}
        {brandField}
      </div>
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
      {values}
      <details className="rounded-lg border p-3" open={optional.some((k) => wrong.has(k))}>
        <summary className={summaryClass}>{t.foods.moreNutrients}</summary>
        <div className="mt-3">{otherValues}</div>
      </details>
      {portionField}
      {servingsField}
      {aliasesField}
      {barcodesField}
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
          {/* Open Food Facts only for a food that came from there. */}
          {sources
            .filter((s) => s !== 'off' || from?.source === 'off')
            .map((s) => (
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
      <Button type="submit" className="w-full" disabled={save.isPending}>
        {t.foods.save}
      </Button>
      {saveStatus}
    </form>
  )
}

// TakenBy says that another food of the user's has a name and brand, or
// an other name, and which, with a link to it.
function TakenBy({ foodId, by }: { foodId: string; by: 'name' | 'alias' | 'barcode' }) {
  const t = useMessages()
  const other = $api.useQuery('get', '/api/v1/foods/{id}', { params: { path: { id: foodId } } })
  const name = other.data
    ? [other.data.name, other.data.brand].filter(Boolean).join(' · ')
    : t.foods.thatFood
  return (
    <>
      {{ name: t.foods.nameTaken, alias: t.foods.aliasTaken, barcode: t.foods.barcodeTaken }[by]}{' '}
      <Link to="/foods/$id" params={{ id: foodId }} className="underline underline-offset-2">
        {name}
      </Link>
      {other.data?.archived && ` (${t.foods.archivedMark})`}
    </>
  )
}
