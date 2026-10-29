import { Check, X } from 'lucide-react'
import { useEffect, useId, useRef, type SubmitEvent } from 'react'

import { units } from '@/components/day/metric-kinds'
import { useMetricValue } from '@/components/day/metric-value'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// WeighIn is a weigh-in of day date in a line: its label, a field in kg
// and ✓, which Enter is too. The field starts with initial (the day's
// weight, as shown: left as it is, it is not sent), and hints with
// placeholder, the weight of before, which a weigh-in is near; focus
// gives it the focus as it shows. Saved, a toast says so, with Undo
// (useMetricValue), and onSaved is told. onCancel adds a way to give it
// up, which Escape takes too.
export function WeighIn({
  date,
  label,
  initial = '',
  placeholder,
  focus = false,
  onSaved,
  onCancel,
  className,
}: {
  date: string
  label: string
  initial?: string
  placeholder?: string | undefined
  focus?: boolean
  onSaved?: () => void
  onCancel?: () => void
  className?: string
}) {
  const t = useMessages()
  const id = useId()
  const field = useRef<HTMLInputElement>(null)
  const weight = useMetricValue(date, 'weight', onSaved)
  useEffect(() => {
    if (focus) {
      field.current?.focus()
    }
  }, [focus])

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const typed = field.current?.value ?? ''
    if (initial !== '' && typed === initial) {
      onSaved?.()
      return
    }
    weight.give(typed)
  }

  return (
    <form onSubmit={submit} noValidate className={cn('space-y-1', className)}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <label htmlFor={id} className="text-small font-medium">
          {label}
        </label>
        <div className="flex items-center gap-2">
          <div className="relative w-28">
            <input
              ref={field}
              id={id}
              name="value"
              inputMode="decimal"
              autoComplete="off"
              defaultValue={initial}
              placeholder={placeholder}
              aria-invalid={weight.invalid}
              aria-describedby={`${id}-unit`}
              className={cn(inputClass, 'pr-10 tabular-nums')}
              onChange={weight.clear}
              onKeyDown={(event) => {
                if (event.key === 'Escape' && onCancel) {
                  onCancel()
                }
              }}
            />
            <span
              id={`${id}-unit`}
              className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-muted-foreground"
            >
              {units.weight}
            </span>
          </div>
          <Button
            type="submit"
            size="icon"
            aria-label={t.day.weighIn.save}
            disabled={weight.log.isPending}
          >
            <Check aria-hidden />
          </Button>
          {onCancel && (
            <Button variant="ghost" size="icon" aria-label={t.day.cancel} onClick={onCancel}>
              <X aria-hidden />
            </Button>
          )}
        </div>
      </div>
      {weight.problem === undefined ? (
        <SaveStatus change={weight.log} onRetry={weight.retry} />
      ) : (
        <p role="alert" className="text-xs text-destructive">
          {weight.problem}
        </p>
      )}
    </form>
  )
}
