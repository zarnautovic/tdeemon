import { Scale } from 'lucide-react'
import { useRef, useState } from 'react'
import { flushSync } from 'react-dom'

import { usePreviousWeight } from '@/components/day/previous-weight'
import { WeighIn } from '@/components/day/weigh-in'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// WeighInChip is a weigh-in of day date from the quick add, beside the
// ways to an estimate and a new food: a chip that opens its field in
// place, hinting with the day before's. Saved, the toast over the sheet
// says so, with Undo, and the chip is back, with the focus.
export function WeighInChip({ date }: { date: string }) {
  const t = useMessages()
  const f = useFormat()
  const [open, setOpen] = useState(false)
  const chip = useRef<HTMLButtonElement>(null)
  const before = usePreviousWeight(date, open)
  const close = () => {
    flushSync(() => {
      setOpen(false)
    })
    chip.current?.focus()
  }

  if (open) {
    return (
      <div className="rounded-lg border p-3">
        <WeighIn
          date={date}
          label={t.day.metric.weight}
          placeholder={before?.weight === undefined ? undefined : f.field(before.weight.value, 1)}
          focus
          onSaved={close}
          onCancel={close}
        />
      </div>
    )
  }
  return (
    <Button
      ref={chip}
      variant="outline"
      className="h-auto min-h-11 w-full border-dashed py-2"
      onClick={() => {
        setOpen(true)
      }}
    >
      <Scale aria-hidden />
      {t.day.weighIn.chip}
    </Button>
  )
}
