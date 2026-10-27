import { Plus } from 'lucide-react'
import { useId, useState } from 'react'

import { MetricForm } from '@/components/day/metric-form'
import type { MetricKind } from '@/components/day/metric-kinds'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// AddMetric adds a measurement of one of kinds, those the day has none
// of, to day date.
export function AddMetric({ date, kinds }: { date: string; kinds: readonly MetricKind[] }) {
  const t = useMessages()
  const id = useId()
  const [open, setOpen] = useState(false)
  const [picked, setPicked] = useState<MetricKind | undefined>(undefined)
  // The kind picked, while the day has none of it yet; else the first.
  const kind = picked !== undefined && kinds.includes(picked) ? picked : kinds[0]

  if (kind === undefined) {
    return null
  }
  if (!open) {
    return (
      <Button
        variant="outline"
        className="w-full"
        onClick={() => {
          setOpen(true)
        }}
      >
        <Plus aria-hidden />
        {t.day.addMetric}
      </Button>
    )
  }
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <div className="space-y-1.5">
        <label htmlFor={id} className={cn(fieldLabel, 'block')}>
          {t.day.kind}
        </label>
        <select
          id={id}
          value={kind}
          className={inputClass}
          onChange={(event) => {
            const chosen = event.currentTarget.value
            setPicked(kinds.find((k) => k === chosen))
          }}
        >
          {kinds.map((k) => (
            <option key={k} value={k}>
              {t.day.metric[k]}
            </option>
          ))}
        </select>
      </div>
      <MetricForm
        // A kind of its own starts the field empty.
        key={kind}
        date={date}
        kind={kind}
        label={t.day.value}
        onSaved={() => {
          setOpen(false)
          setPicked(undefined)
        }}
      >
        <Button
          variant="ghost"
          onClick={() => {
            setOpen(false)
          }}
        >
          {t.day.cancel}
        </Button>
      </MetricForm>
    </div>
  )
}
