import { useEffect, useId, useRef, useState } from 'react'

import { useCheckIn } from '@/api/day'
import type { Day } from '@/api/types'
import { Field } from '@/components/day/field'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { inputClass, linkTarget } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

// maxNote is the server's bound on a note, in characters
// (day.MaxNoteLen).
const maxNote = 2000

// DayNote is what the user says of the day in words, saved when the field
// is left. What is typed stays in the field until it is saved, though the
// day comes back (with an earlier change) in between. A day without a
// note has a link for one, as most days need none.
export function DayNote({ day }: { day: Day }) {
  const t = useMessages()
  const id = useId()
  const change = useCheckIn(day.date, () => {
    sayChange(
      { title: t.toast.dayNote },
      { last: { action: 'day.checkin', entity: day.date }, date: day.date },
    )
  })
  const note = day.note ?? ''
  // What is typed and not saved yet; undefined, the day's note.
  const [draft, setDraft] = useState<string | undefined>(undefined)
  // The field, once asked for, takes the focus.
  const [asked, setAsked] = useState(false)
  const field = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    if (asked) {
      field.current?.focus()
    }
  }, [asked])

  // save sends what is typed, typed, and once it is saved the field shows
  // the day's note again, unless more was typed meanwhile.
  function save(typed: string) {
    // The server keeps no space at a note's ends (a keyboard's suggestion
    // leaves one).
    const text = typed.trim()
    if (text === note) {
      setDraft((now) => (now === typed ? undefined : now))
      return
    }
    change.mutate(
      { params: { path: { date: day.date } }, body: { note: text } },
      {
        onSuccess: () => {
          setDraft((now) => (now === typed ? undefined : now))
        },
      },
    )
  }

  if (!asked && note === '' && draft === undefined) {
    return (
      <Button
        variant="link"
        className={cn(linkTarget, 'h-auto p-0 text-small font-semibold')}
        onClick={() => {
          setAsked(true)
        }}
      >
        + {t.day.addNote}
      </Button>
    )
  }

  return (
    <Field
      label={t.day.note}
      htmlFor={id}
      status={
        <SaveStatus
          change={change}
          // Again, with what is typed now.
          onRetry={() => {
            if (draft !== undefined) {
              save(draft)
            }
          }}
        />
      }
    >
      <textarea
        ref={field}
        id={id}
        value={draft ?? note}
        maxLength={maxNote}
        rows={2}
        placeholder={t.day.notePlaceholder}
        className={cn(inputClass, 'h-auto py-2')}
        onChange={(event) => {
          setDraft(event.currentTarget.value)
        }}
        onBlur={() => {
          if (draft !== undefined) {
            save(draft)
          }
        }}
      />
    </Field>
  )
}
