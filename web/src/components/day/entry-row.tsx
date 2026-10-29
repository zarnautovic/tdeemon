import { X } from 'lucide-react'
import { useEffect, useRef } from 'react'

import { AmountField } from '@/components/day/amount-field'
import { EstimateFields } from '@/components/day/estimate-fields'
import { inUnit, servingOf, unitsOf, type Entry } from '@/components/day/meal-entries'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// EntryRow is an item of a meal being logged, to change or remove: a
// food's amount and unit, or an estimate's name and values. wrong marks
// the fields that cannot be sent as they are, which describedBy says why
// of; focus takes the focus to its first field when it comes.
export function EntryRow({
  entry,
  wrong,
  describedBy,
  focus,
  onChange,
  onRemove,
}: {
  entry: Entry
  wrong: boolean
  describedBy: string | undefined
  focus: boolean
  onChange: (entry: Entry) => void
  onRemove: () => void
}) {
  const t = useMessages()
  const f = useFormat()
  const firstField = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (focus) {
      firstField.current?.focus()
    }
  }, [focus])
  const brand = entry.kind === 'food' && entry.food.brand ? ` · ${entry.food.brand}` : ''
  const name = entry.kind === 'food' ? entry.food.name + brand : entry.name || t.log.anEstimate
  const remove = (
    <Button variant="ghost" size="icon" aria-label={t.log.remove(name)} onClick={onRemove}>
      <X aria-hidden />
    </Button>
  )

  if (entry.kind === 'food') {
    return (
      <li className="flex items-center gap-2 py-2">
        <span className="min-w-0 flex-1 text-sm wrap-break-word">
          {entry.food.name}
          {brand && <span className="text-muted-foreground">{brand}</span>}
        </span>
        <AmountField
          ref={firstField}
          aria-label={t.log.amount(name)}
          aria-describedby={describedBy}
          value={entry.amount}
          // A food's entry is wrong by its amount alone.
          aria-invalid={wrong}
          onChange={(e) => {
            // Typed: what a unit chosen worked out is no more.
            const { exact: _typed, ...rest } = entry
            onChange({ ...rest, amount: e.currentTarget.value })
          }}
          select={{
            'aria-label': t.log.unit(name),
            value: entry.unit,
            onChange: (e) => {
              // The amount is worked out anew in the unit chosen, so the
              // item weighs what it did.
              onChange(inUnit(entry, e.currentTarget.value, f.parse, f.field))
            },
            children: unitsOf(entry.food).map((u) => (
              <option key={u} value={u}>
                {u === 'portion'
                  ? t.log.portions(f.parse(entry.amount) ?? Number.NaN)
                  : (servingOf(u) ?? u)}
              </option>
            )),
          }}
        />
        {remove}
      </li>
    )
  }
  return (
    <li className="py-2">
      <EstimateFields
        nameRef={firstField}
        entry={entry}
        wrong={wrong}
        describedBy={describedBy}
        onChange={onChange}
      >
        {remove}
      </EstimateFields>
    </li>
  )
}
