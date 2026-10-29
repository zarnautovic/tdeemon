import { useIsMutating } from '@tanstack/react-query'
import { ChevronLeft, ExternalLink } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import { fieldsOf, useSaveFood } from '@/api/foods'
import type { components } from '@/api/schema'
import type { Food } from '@/api/types'
import { Amount } from '@/components/day/finder'
import {
  entryWrong,
  foodEntry,
  unitsOf,
  type Entry,
  type FoodEntry,
  type Wrong,
} from '@/components/day/meal-entries'
import { SaveStatus } from '@/components/day/save-status'
import { Scanner } from '@/components/day/scanner'
import { FoodForm } from '@/components/foods/food-form'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { uuidv7 } from '@/lib/uuid'

type FoodFields = components['schemas']['FoodFields']

// ScanEntry is the quick add's step of a barcode, in the search's place:
// the camera reads it, or its digits are typed, and the server says what
// it is (lookup-barcode). The user's food of it opens for its amount; a
// product of Open Food Facts opens as a new food, its values to see and
// correct, saved with the barcode as it is added; the user's food of the
// product's name, which lacks the barcode, gets it, and opens for its
// amount; a barcode nobody has opens a new food of it, its values to
// type off the label. A barcode misread is read again. onBack goes back
// to the search, and onAdd takes the entry.
export function ScanEntry({
  onBack,
  onAdd,
}: {
  onBack: () => void
  onAdd: (entry: Entry) => void
}) {
  const t = useMessages()
  const [code, setCode] = useState<string | undefined>(undefined)

  return (
    <>
      <div className="space-y-1 px-4 pt-2">
        <Button variant="ghost" size="sm" className="-ml-2.5" onClick={onBack}>
          <ChevronLeft aria-hidden />
          {t.log.back}
        </Button>
        <h3 className="text-h3">{t.scan.title}</h3>
      </div>
      {code === undefined ? (
        <div className="min-h-0 flex-1 overflow-y-auto p-4">
          <Scanner onCode={setCode} />
        </div>
      ) : (
        <Found
          code={code}
          onAgain={() => {
            setCode(undefined)
          }}
          onAdd={onAdd}
        />
      )}
    </>
  )
}

// Found is what the barcode code is, as the server says.
function Found({
  code,
  onAgain,
  onAdd,
}: {
  code: string
  onAgain: () => void
  onAdd: (entry: Entry) => void
}) {
  const t = useMessages()
  const lookup = $api.useQuery(
    'get',
    '/api/v1/barcodes/{code}',
    { params: { path: { code } } },
    { retry: false, staleTime: Infinity },
  )
  // The user's food that got the barcode, ready for its amount.
  const [linked, setLinked] = useState<Food | undefined>(undefined)

  if (lookup.isPending) {
    return <Body>{t.scan.looking(code)}</Body>
  }
  if (lookup.isError) {
    const status = statusOf(lookup.error)
    if (status === 404) {
      return <NewFood start={{ barcodes: [code] }} said={t.scan.unknown(code)} onAdd={onAdd} />
    }
    return (
      <Body>
        <p role="alert" className="text-sm text-destructive">
          {status === 422 ? t.scan.misread(code) : t.scan.failed(detailOf(lookup.error))}
        </p>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={onAgain}>
            {t.scan.again}
          </Button>
          {status !== 422 && (
            <Button variant="outline" onClick={() => void lookup.refetch()}>
              {t.retry}
            </Button>
          )}
        </div>
      </Body>
    )
  }
  const { food, named_food: named, open_food_facts: product } = lookup.data
  const yours = food ?? linked
  if (yours !== undefined) {
    return <FoodAmount food={yours} onAdd={onAdd} />
  }
  if (named !== undefined) {
    return <Named food={named} code={code} onLinked={setLinked} />
  }
  if (product !== undefined) {
    return (
      <NewFood
        start={product.food}
        said={
          <>
            {t.scan.fromOpenFoodFacts}{' '}
            <a
              href={product.url}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-primary underline underline-offset-2"
            >
              Open Food Facts
              <ExternalLink aria-hidden className="size-3.5" />
            </a>
            . {t.scan.checkLabel}
          </>
        }
        onAdd={onAdd}
      />
    )
  }
  return null
}

// Body is the step's scrolled part.
function Body({ children }: { children: React.ReactNode }) {
  return (
    <div aria-live="polite" className="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
      {children}
    </div>
  )
}

// FoodAmount is the user's food of the barcode, open for its amount.
function FoodAmount({ food, onAdd }: { food: Food; onAdd: (entry: Entry) => void }) {
  const t = useMessages()
  const f = useFormat()
  const [entry, setEntry] = useState<FoodEntry>(() => foodEntry(food, f.field))
  const [wrong, setWrong] = useState<Wrong | undefined>(undefined)

  return (
    <Body>
      <p className="font-medium">
        {food.brand ? `${food.name} · ${food.brand}` : food.name}
        <span className="block text-caption font-normal text-muted-foreground">
          {t.scan.yours} · {t.unit.kcal(f.number(food.nutrients.kcal))} {t.foods.per[food.basis]}
        </span>
      </p>
      <Amount
        entry={entry}
        wrong={wrong}
        onChange={(e) => {
          setEntry(e)
          setWrong(undefined)
        }}
        onAdd={(e) => {
          const why = entryWrong(e, f.parse)
          if (why !== undefined) {
            setWrong(why)
            return
          }
          onAdd(e)
        }}
      />
    </Body>
  )
}

// Named is the user's food of the product's name and brand, which lacks
// the barcode: added to it, it is the barcode's food, ready for its
// amount (onLinked).
function Named({
  food,
  code,
  onLinked,
}: {
  food: Food
  code: string
  onLinked: (food: Food) => void
}) {
  const t = useMessages()
  const save = useSaveFood()
  const button = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    button.current?.focus()
  }, [])

  return (
    <Body>
      <p>{t.scan.named(food.brand ? `${food.name} · ${food.brand}` : food.name)}</p>
      <Button
        ref={button}
        disabled={save.isPending}
        onClick={() => {
          save.mutate(
            { body: { ...fieldsOf(food), barcodes: [...(food.barcodes ?? []), code] } },
            { onSuccess: onLinked },
          )
        }}
      >
        {t.scan.link}
      </Button>
      <SaveStatus
        change={save}
        onRetry={() => {
          if (save.variables) {
            save.mutate(save.variables, { onSuccess: onLinked })
          }
        }}
      />
    </Body>
  )
}

// NewFood is a new food of the barcode, made in short (FoodForm's compact
// form) from start, with what said says of where it came from, and added
// in the amount eaten now.
function NewFood({
  start,
  said,
  onAdd,
}: {
  start: Partial<FoodFields>
  said: React.ReactNode
  onAdd: (entry: Entry) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const form = `${id}-form`
  // A food on its way: Add to meal waits for it.
  const saving = useIsMutating({ mutationKey: ['post', '/api/v1/foods/new'] }) > 0

  return (
    <>
      <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
        <p className="rounded-lg bg-surface-2 p-3 text-sm text-muted-foreground">{said}</p>
        <FoodForm
          compact={{
            id: form,
            name: '',
            start,
            onAdd: (food, amount) => {
              const unit = unitsOf(food)[0] ?? 'g'
              onAdd({ key: uuidv7(), kind: 'food', food, amount: f.field(amount), unit })
            },
          }}
        />
      </div>
      <div className="border-t p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <Button
          type="submit"
          form={form}
          className="h-13 w-full text-base"
          disabled={saving}
          focusableWhenDisabled
        >
          {t.log.addToMeal}
        </Button>
      </div>
    </>
  )
}
