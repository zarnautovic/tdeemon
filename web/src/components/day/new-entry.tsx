import { useIsMutating } from '@tanstack/react-query'
import { ChevronLeft } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'

import { EstimateFields } from '@/components/day/estimate-fields'
import {
  entryWrong,
  sayWrong,
  unitsOf,
  type Entry,
  type Wrong,
} from '@/components/day/meal-entries'
import { FoodForm } from '@/components/foods/food-form'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { uuidv7 } from '@/lib/uuid'

// What the next step of the quick add makes: a food, an estimate, or an
// entry of a barcode (ScanEntry).
export type Making = 'food' | 'estimate' | 'scan'

// NewEntry is the quick add's next step, in the search's place: a food of
// the user's made in short (FoodForm's compact form) and added in the
// amount eaten now, or an estimate of a plate, its name and values; each
// added with the one button at its foot, Add to meal. name is what was
// searched for, which the food or the estimate is named to start with;
// onBack goes back to the search, and onAdd takes the entry, checked.
export function NewEntry({
  making,
  name,
  onBack,
  onAdd,
}: {
  making: Exclude<Making, 'scan'>
  name: string
  onBack: () => void
  onAdd: (entry: Entry) => void
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const form = `${id}-form`
  const body = useRef<HTMLDivElement>(null)
  const [estimate, setEstimate] = useState<Entry & { kind: 'estimate' }>(() => ({
    key: uuidv7(),
    kind: 'estimate',
    name,
    kcal: '',
    protein: '',
    carbs: '',
    fat: '',
  }))
  const [wrong, setWrong] = useState<Wrong | undefined>(undefined)
  // A food on its way: Add to meal waits for it.
  const saving = useIsMutating({ mutationKey: ['post', '/api/v1/foods/new'] }) > 0
  // The focus goes to the first field, as the button that opened the step
  // is hidden under it.
  useEffect(() => {
    body.current?.querySelector<HTMLElement>('input')?.focus()
  }, [])

  return (
    <>
      <div className="space-y-1 px-4 pt-2">
        <Button variant="ghost" size="sm" className="-ml-2.5" onClick={onBack}>
          <ChevronLeft aria-hidden />
          {t.log.back}
        </Button>
        <h3 className="text-h3">{making === 'food' ? t.log.newFood : t.log.anEstimateOf}</h3>
      </div>
      <div ref={body} className="min-h-0 flex-1 overflow-y-auto p-4">
        {making === 'food' ? (
          <FoodForm
            compact={{
              id: form,
              name,
              // In the unit of what its values are for.
              onAdd: (food, amount) => {
                const unit = unitsOf(food)[0] ?? 'g'
                onAdd({ key: uuidv7(), kind: 'food', food, amount: f.field(amount), unit })
              },
            }}
          />
        ) : (
          <form
            id={form}
            noValidate
            className="space-y-2"
            onSubmit={(e) => {
              e.preventDefault()
              const why = entryWrong(estimate, f.parse)
              if (why !== undefined) {
                setWrong(why)
                return
              }
              onAdd(estimate)
            }}
          >
            <EstimateFields
              entry={estimate}
              wrong={wrong !== undefined}
              describedBy={wrong ? `${id}-wrong` : undefined}
              onChange={(e) => {
                if (e.kind === 'estimate') {
                  setEstimate(e)
                }
              }}
            />
            {wrong && (
              <p id={`${id}-wrong`} role="alert" className="text-sm text-destructive">
                {sayWrong(wrong, t, f.number)}
              </p>
            )}
          </form>
        )}
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
