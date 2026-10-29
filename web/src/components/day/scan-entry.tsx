import { ChevronLeft, ExternalLink } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import { fieldsOf, useSaveFood } from '@/api/foods'
import type { Food } from '@/api/types'
import { Amount } from '@/components/day/finder'
import {
  entryWrong,
  foodEntry,
  type Entry,
  type FoodEntry,
  type Wrong,
} from '@/components/day/meal-entries'
import { NewEntry } from '@/components/day/new-entry'
import { SaveStatus } from '@/components/day/save-status'
import { Scanner } from '@/components/day/scanner'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { sayChange } from '@/lib/toast'

// nameOf is a food's name, with its brand.
function nameOf(food: Food): string {
  return food.brand ? `${food.name} · ${food.brand}` : food.name
}

// ScanEntry is the quick add's step of a barcode, in the search's place:
// the camera reads it, or its digits are typed, and the server says what
// it is (lookup-barcode), asked anew each time. The user's food of it
// opens for its amount, the last eaten; a product of Open Food Facts
// opens as a new food (NewEntry), its values to check against the label,
// saved with the barcode as it is added; the user's food of the
// product's name, which lacks the barcode, gets it in a tap, and opens
// for its amount; a barcode nobody has opens a new food of it, its values
// typed off the label, as one does when the lookup fails. A barcode
// misread is read again. onBack goes back to the search, and onAdd takes
// the entry.
export function ScanEntry({
  onBack,
  onAdd,
}: {
  onBack: () => void
  onAdd: (entry: Entry) => void
}) {
  const t = useMessages()
  const [code, setCode] = useState<string | undefined>(undefined)
  // A food made of the barcode though the lookup failed.
  const [making, setMaking] = useState(false)
  // The user's food that got the barcode, ready for its amount.
  const [linked, setLinked] = useState<Food | undefined>(undefined)
  const lookup = $api.useQuery(
    'get',
    '/api/v1/barcodes/{code}',
    { params: { path: { code: code ?? '' } } },
    // Asked anew each scan: a food made of it since is the user's.
    { enabled: code !== undefined, retry: false, gcTime: 0, refetchOnWindowFocus: false },
  )
  // The step's heading takes the focus, as the button that opened it is
  // hidden under it, and again with each code, read or typed (null: not
  // yet).
  const heading = useRef<HTMLHeadingElement>(null)
  const focusedAt = useRef<string | undefined | null>(null)
  useEffect(() => {
    if (focusedAt.current !== code) {
      focusedAt.current = code
      heading.current?.focus()
    }
  })

  const status = statusOf(lookup.error)
  const data = code === undefined ? undefined : lookup.data
  const product =
    data?.food === undefined && data?.named_food === undefined ? data?.open_food_facts : undefined
  if (code !== undefined && (making || status === 404)) {
    return (
      <NewEntry
        making="food"
        name=""
        start={{ barcodes: [code] }}
        said={t.scan.unknown(code)}
        onBack={onBack}
        onAdd={onAdd}
      />
    )
  }
  if (product !== undefined) {
    return (
      <NewEntry
        making="food"
        name=""
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
        onBack={onBack}
        onAdd={onAdd}
      />
    )
  }

  const again = () => {
    setCode(undefined)
    setLinked(undefined)
  }
  let body: React.ReactNode
  if (code === undefined) {
    body = <Scanner onCode={setCode} />
  } else if (lookup.isPending) {
    body = <p>{t.scan.looking(code)}</p>
  } else if (lookup.isError) {
    body = (
      <>
        <p role="alert" className="text-sm text-destructive">
          {status === 422 ? t.scan.misread(code) : t.scan.failed(detailOf(lookup.error))}
        </p>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={again}>
            {t.scan.again}
          </Button>
          {status !== 422 && (
            <>
              <Button variant="outline" onClick={() => void lookup.refetch()}>
                {t.retry}
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setMaking(true)
                }}
              >
                {t.scan.makeIt}
              </Button>
            </>
          )}
        </div>
      </>
    )
  } else {
    const yours = data?.food ?? linked
    const named = data?.named_food
    if (yours !== undefined) {
      body = <FoodAmount key={yours.id} food={yours} onAdd={onAdd} />
    } else if (named !== undefined) {
      body = <Named food={named} code={code} onLinked={setLinked} />
    }
  }

  return (
    <>
      <div className="space-y-1 px-4 pt-2">
        <Button variant="ghost" size="sm" className="-ml-2.5" onClick={onBack}>
          <ChevronLeft aria-hidden />
          {t.log.back}
        </Button>
        <h3 ref={heading} tabIndex={-1} className="text-h3 outline-none">
          {t.scan.title}
        </h3>
      </div>
      <div aria-live="polite" className="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
        {body}
      </div>
    </>
  )
}

// FoodAmount is the user's food of the barcode, open for its amount.
function FoodAmount({ food, onAdd }: { food: Food; onAdd: (entry: Entry) => void }) {
  const t = useMessages()
  const f = useFormat()
  const [entry, setEntry] = useState<FoodEntry>(() => foodEntry(food, f.field))
  const [wrong, setWrong] = useState<Wrong | undefined>(undefined)

  return (
    <>
      <p className="font-medium">
        {nameOf(food)}
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
    </>
  )
}

// Named is the user's food of the product's name and brand, which lacks
// the barcode: added to it, as a change the toast takes back, it is the
// barcode's food, in its last use, ready for its amount (onLinked).
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
  const linked = (saved: Food) => {
    sayChange(
      { title: t.toast.savedOf(saved.name) },
      // Of no day: after the changes of today's still on their way.
      { last: { action: 'food.save', entity: saved.id }, date: 'today' },
    )
    onLinked({ ...saved, ...(food.last_use === undefined ? {} : { last_use: food.last_use }) })
  }

  return (
    <>
      <p>{t.scan.named(nameOf(food))}</p>
      <Button
        disabled={save.isPending}
        onClick={() => {
          save.mutate(
            { body: { ...fieldsOf(food), barcodes: [...(food.barcodes ?? []), code] } },
            { onSuccess: linked },
          )
        }}
      >
        {t.scan.link}
      </Button>
      <SaveStatus
        change={save}
        onRetry={() => {
          if (save.variables) {
            save.mutate(save.variables, { onSuccess: linked })
          }
        }}
      />
    </>
  )
}
