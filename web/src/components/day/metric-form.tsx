import { useId, type ReactNode, type Ref, type SubmitEvent } from 'react'

import { units, type MetricKind } from '@/components/day/metric-kinds'
import { useMetricValue } from '@/components/day/metric-value'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// MetricForm takes a measurement of a kind, typed in the language and the
// kind's unit, and stores it for the day; what is not a number, or not a
// value of the kind, it says so of, and sends nothing, until the field
// changes (useMetricValue). Saved, a toast says so, by the kind's name,
// whatever label names the field, with Undo. initial is what the field
// starts with, as the value is shown: left as it is, it is not sent, so
// the value keeps the decimals it was shown without. inputRef is the
// field's, for a sheet opened at it to focus. children are more buttons,
// after Save.
export function MetricForm({
  date,
  kind,
  label,
  initial = '',
  inputRef,
  onSaved,
  children,
}: {
  date: string
  kind: MetricKind
  label: string
  initial?: string
  inputRef?: Ref<HTMLInputElement> | undefined
  onSaved?: () => void
  children?: ReactNode
}) {
  const t = useMessages()
  const id = useId()
  const value = useMetricValue(date, kind, onSaved)
  const unit = units[kind]

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const typed = new FormData(event.currentTarget).get('value')
    if (initial !== '' && typed === initial) {
      onSaved?.()
      return
    }
    value.give(typeof typed === 'string' ? typed : '')
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-1.5">
      <label htmlFor={id} className={cn(fieldLabel, 'block')}>
        {label}
      </label>
      <div className="flex flex-wrap gap-2">
        <div className="relative min-w-32 flex-1">
          <input
            ref={inputRef}
            id={id}
            name="value"
            inputMode="decimal"
            autoComplete="off"
            defaultValue={initial}
            aria-invalid={value.invalid}
            aria-describedby={unit ? `${id}-unit` : undefined}
            className={cn(inputClass, 'tabular-nums', unit && 'pr-20')}
            onChange={value.clear}
          />
          {unit && (
            <span
              id={`${id}-unit`}
              className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-muted-foreground"
            >
              {unit}
            </span>
          )}
        </div>
        <Button type="submit" disabled={value.log.isPending}>
          {t.day.save}
        </Button>
        {children}
      </div>
      {value.problem === undefined ? (
        <SaveStatus change={value.log} onRetry={value.retry} />
      ) : (
        <p role="alert" className="text-xs text-destructive">
          {value.problem}
        </p>
      )}
    </form>
  )
}
