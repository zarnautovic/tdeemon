import { useId } from 'react'

import { useCheckIn } from '@/api/day'
import type { Day } from '@/api/types'
import { Field } from '@/components/day/field'
import { SaveStatus } from '@/components/day/save-status'
import { useMessages } from '@/i18n/messages'

const choices = ['training', 'rest', 'plan'] as const

type Choice = (typeof choices)[number]

// choiceOf is the day's type as the user chose it: plan when the plan's
// training weekdays decide.
function choiceOf(day: Day): Choice {
  if (day.training_toggle === undefined) {
    return 'plan'
  }
  return day.training_toggle ? 'training' : 'rest'
}

// DayTypePicker sets the day's type, which picks its target: training,
// rest, or as the plan has it.
export function DayTypePicker({ day }: { day: Day }) {
  const t = useMessages()
  const id = useId()
  const change = useCheckIn(day.date)
  const sent = change.variables?.body.training_day
  const chosen = (change.isPending || change.isError) && sent !== undefined ? sent : choiceOf(day)

  return (
    <Field label={t.day.dayType} labelId={id} status={<SaveStatus change={change} />}>
      <fieldset aria-labelledby={id} className="flex min-w-0 gap-1">
        {choices.map((choice) => (
          <button
            key={choice}
            type="button"
            aria-pressed={chosen === choice}
            onClick={() => {
              if (chosen !== choice) {
                change.mutate({
                  params: { path: { date: day.date } },
                  body: { training_day: choice },
                })
              }
            }}
            className="min-h-10 flex-1 rounded-lg border bg-muted/40 px-1 text-sm text-muted-foreground transition-colors outline-none hover:border-muted-foreground focus-visible:ring-3 focus-visible:ring-ring/50 aria-pressed:border-primary aria-pressed:bg-primary aria-pressed:text-primary-foreground"
          >
            {t.day.dayTypes[choice]}
          </button>
        ))}
      </fieldset>
    </Field>
  )
}
