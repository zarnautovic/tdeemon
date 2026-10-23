import { useId } from 'react'

import { useCheckIn } from '@/api/day'
import type { Day } from '@/api/types'
import { Field } from '@/components/day/field'
import { SaveStatus } from '@/components/day/save-status'
import { useMessages } from '@/i18n/messages'
import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// maxNote is the server's bound on a note, in characters
// (day.MaxNoteLen).
const maxNote = 2000

// DayNote is what the user says of the day in words, saved when the field
// is left.
export function DayNote({ day }: { day: Day }) {
  const t = useMessages()
  const id = useId()
  const change = useCheckIn(day.date)
  const note = day.note ?? ''

  return (
    <Field label={t.day.note} htmlFor={id} status={<SaveStatus change={change} />}>
      <textarea
        id={id}
        // A note the day comes back with (saved, or changed elsewhere)
        // starts the field again.
        key={note}
        defaultValue={note}
        maxLength={maxNote}
        rows={2}
        placeholder={t.day.notePlaceholder}
        className={cn(inputClass, 'h-auto py-2')}
        onBlur={(event) => {
          const typed = event.currentTarget.value
          if (typed !== note) {
            change.mutate({ params: { path: { date: day.date } }, body: { note: typed } })
          }
        }}
      />
    </Field>
  )
}
