import { useId, useState, type ReactNode, type SubmitEvent } from 'react'

import { useLogMetric } from '@/api/day'
import { statusOf } from '@/api/errors'
import { units, type MetricKind } from '@/components/day/metric-kinds'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { fieldLabel, inputClass } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

// MetricForm takes a measurement of a kind, typed in the language and the
// kind's unit, and stores it for the day; what is not a number, or not a
// value of the kind (limits.json, the server's bounds), it says so of,
// and sends nothing, until the field changes. Saved, a toast says so,
// with Undo. children are more buttons, after Save.
export function MetricForm({
  date,
  kind,
  label,
  initial = '',
  onSaved,
  children,
}: {
  date: string
  kind: MetricKind
  label: string
  initial?: string
  onSaved?: () => void
  children?: ReactNode
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const log = useLogMetric(date, () => {
    sayChange(
      { title: t.toast.savedOf(label) },
      { last: { action: 'metric.set', entity: `${date}/${kind}` }, date },
    )
  })
  const [notNumber, setNotNumber] = useState(false)
  const [outside, setOutside] = useState(false)
  const unit = units[kind]
  const { least, most } = limits.metrics[kind]
  // A value the server refuses (422) is out of what the kind can be:
  // sending it again would not help.
  const refused = log.isError && statusOf(log.error) === 422

  function save(value: number) {
    log.mutate(
      { params: { path: { date, kind } }, body: { value } },
      { onSuccess: () => onSaved?.() },
    )
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const typed = new FormData(event.currentTarget).get('value')
    const value = typeof typed === 'string' ? f.parse(typed) : undefined
    const out = value !== undefined && (value < least || value > most)
    setNotNumber(value === undefined)
    setOutside(out)
    if (value !== undefined && !out) {
      save(value)
    }
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-1.5">
      <label htmlFor={id} className={cn(fieldLabel, 'block')}>
        {label}
      </label>
      <div className="flex flex-wrap gap-2">
        <div className="relative min-w-32 flex-1">
          <input
            id={id}
            name="value"
            inputMode="decimal"
            autoComplete="off"
            defaultValue={initial}
            aria-invalid={notNumber || outside || log.isError}
            aria-describedby={unit ? `${id}-unit` : undefined}
            className={cn(inputClass, 'tabular-nums', unit && 'pr-20')}
            onChange={() => {
              setNotNumber(false)
              setOutside(false)
              if (log.isError) {
                log.reset()
              }
            }}
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
        <Button type="submit" disabled={log.isPending}>
          {t.day.save}
        </Button>
        {children}
      </div>
      {notNumber || outside || refused ? (
        <p role="alert" className="text-xs text-destructive">
          {notNumber
            ? t.day.notANumber
            : outside
              ? t.day.between(f.number(least, 1), f.number(most, 1), unit)
              : t.day.outOfRange}
        </p>
      ) : (
        <SaveStatus
          change={log}
          onRetry={() => {
            if (log.variables) {
              save(log.variables.body.value)
            }
          }}
        />
      )}
    </form>
  )
}
