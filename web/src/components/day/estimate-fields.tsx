import type { ReactNode, Ref } from 'react'

import type { Entry } from '@/components/day/meal-entries'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

const values = ['kcal', 'protein', 'carbs', 'fat'] as const

// EstimateFields are an estimate's name and its four values, in a grid of
// two by two; beside its name, children (a way to remove it). wrong marks
// those that cannot be sent, which describedBy says why of; nameRef is
// its name's field.
export function EstimateFields({
  entry,
  wrong,
  describedBy,
  nameRef,
  onChange,
  children,
}: {
  entry: Entry & { kind: 'estimate' }
  wrong: boolean
  describedBy: string | undefined
  nameRef?: Ref<HTMLInputElement>
  onChange: (entry: Entry) => void
  children?: ReactNode
}) {
  const t = useMessages()
  const f = useFormat()
  // A value is wrong when the estimate is and it holds no number, or
  // energy past the most an estimate may have.
  const wrongValue = (v: (typeof values)[number]) => {
    const n = f.parse(entry[v])
    return n === undefined || (v === 'kcal' && n > limits.meal.maxEstimateKcal)
  }

  return (
    <div className="space-y-2">
      <div className="flex items-end gap-2">
        <label className="flex-1 space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.log.estimateName}</span>
          <input
            ref={nameRef}
            value={entry.name}
            autoComplete="off"
            aria-invalid={wrong && entry.name.trim() === ''}
            aria-describedby={describedBy}
            className={inputClass}
            onChange={(e) => {
              onChange({ ...entry, name: e.currentTarget.value })
            }}
          />
        </label>
        {children}
      </div>
      <div className="grid grid-cols-2 gap-2">
        {values.map((v) => (
          <label key={v} className="space-y-1">
            <span className={cn(fieldLabel, 'block')}>{t.foods.nutrient[v]}</span>
            <input
              value={entry[v]}
              inputMode="decimal"
              autoComplete="off"
              aria-invalid={wrong && wrongValue(v)}
              aria-describedby={describedBy}
              className={cn(inputClass, 'tabular-nums')}
              onChange={(e) => {
                onChange({ ...entry, [v]: e.currentTarget.value })
              }}
            />
          </label>
        ))}
      </div>
    </div>
  )
}
