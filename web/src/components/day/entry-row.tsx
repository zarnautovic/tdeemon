import { X } from 'lucide-react'

import { servingOf, unitsOf, type Entry } from '@/components/day/meal-entries'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

const values = ['kcal', 'protein', 'carbs', 'fat'] as const

// EntryRow is an item of a meal being logged, to change or remove: a
// food's amount and unit, or an estimate's name and values. wrong marks
// it when it cannot be sent as it is.
export function EntryRow({
  entry,
  wrong,
  onChange,
  onRemove,
}: {
  entry: Entry
  wrong: boolean
  onChange: (entry: Entry) => void
  onRemove: () => void
}) {
  const t = useMessages()
  const name = entry.kind === 'food' ? entry.food.name : entry.name || t.log.anEstimate
  const remove = (
    <Button variant="ghost" size="icon-lg" aria-label={t.log.remove(name)} onClick={onRemove}>
      <X aria-hidden />
    </Button>
  )

  if (entry.kind === 'food') {
    return (
      <li className="flex flex-wrap items-center gap-2 py-2">
        <span className="min-w-0 flex-1 text-sm">{entry.food.name}</span>
        <input
          aria-label={t.log.amount(name)}
          value={entry.amount}
          inputMode="decimal"
          autoComplete="off"
          aria-invalid={wrong}
          className={cn(inputClass, 'h-9 w-20 tabular-nums')}
          onChange={(e) => {
            onChange({ ...entry, amount: e.currentTarget.value })
          }}
        />
        <select
          aria-label={t.log.unit(name)}
          value={entry.unit}
          className={cn(inputClass, 'h-9 w-auto max-w-32')}
          onChange={(e) => {
            onChange({ ...entry, unit: e.currentTarget.value })
          }}
        >
          {unitsOf(entry.food).map((u) => (
            <option key={u} value={u}>
              {u === 'portion' ? t.log.portions : (servingOf(u) ?? u)}
            </option>
          ))}
        </select>
        {remove}
      </li>
    )
  }
  return (
    <li className="space-y-2 py-2">
      <div className="flex items-end gap-2">
        <label className="flex-1 space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.log.estimateName}</span>
          <input
            value={entry.name}
            autoComplete="off"
            aria-invalid={wrong && entry.name.trim() === ''}
            className={inputClass}
            onChange={(e) => {
              onChange({ ...entry, name: e.currentTarget.value })
            }}
          />
        </label>
        {remove}
      </div>
      <div className="grid grid-cols-2 gap-2">
        {values.map((v) => (
          <label key={v} className="space-y-1">
            <span className={cn(fieldLabel, 'block')}>{t.foods.nutrient[v]}</span>
            <input
              value={entry[v]}
              inputMode="decimal"
              autoComplete="off"
              aria-invalid={wrong}
              className={cn(inputClass, 'tabular-nums')}
              onChange={(e) => {
                onChange({ ...entry, [v]: e.currentTarget.value })
              }}
            />
          </label>
        ))}
      </div>
    </li>
  )
}
