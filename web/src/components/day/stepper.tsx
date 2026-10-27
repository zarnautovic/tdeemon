import { useId, type ReactNode } from 'react'

import { Field } from '@/components/day/field'
import type { Scale } from '@/i18n/en'
import { cn } from '@/lib/utils'

// A chosen step's colours: the brand's green for the mood (up is good),
// the warning's amber for hunger (a signal, not a fault: in a deficit
// hunger is expected).
const tones = {
  good: 'aria-pressed:border-success aria-pressed:bg-primary-soft aria-pressed:text-success',
  signal: 'aria-pressed:border-warning aria-pressed:bg-warning/10 aria-pressed:text-warning',
}

// Stepper picks a value of a check-in's scale, 1 to 5, in one tap, each
// step with its word; tapping the chosen step again clears it (0).
export function Stepper({
  label,
  words,
  value,
  tone,
  status,
  onPick,
}: {
  label: string
  words: Scale
  value: number | undefined
  tone: keyof typeof tones
  // status is where the last pick stands, next to the label.
  status: ReactNode
  onPick: (value: number) => void
}) {
  const id = useId()

  return (
    <Field label={label} labelId={id} status={status}>
      <fieldset aria-labelledby={id} className="flex min-w-0 gap-1">
        {words.map((word, i) => {
          const step = i + 1
          return (
            <button
              key={step}
              type="button"
              aria-pressed={value === step}
              onClick={() => {
                onPick(value === step ? 0 : step)
              }}
              className={cn(
                'min-h-11 flex-1 rounded-md border bg-muted/40 px-0.5 py-1.5 text-muted-foreground transition-colors outline-none hover:border-muted-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card',
                tones[tone],
              )}
            >
              <b className="block text-base leading-tight font-bold tabular-nums">{step}</b>{' '}
              <span className="block text-[0.625rem] leading-tight">{word}</span>
            </button>
          )
        })}
      </fieldset>
    </Field>
  )
}
