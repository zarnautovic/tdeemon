import { useId, useState, type SubmitEvent } from 'react'

import { useCheckIn } from '@/api/day'
import type { Day } from '@/api/types'
import { Field } from '@/components/day/field'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { reasonOf, useMessages } from '@/i18n/messages'
import { inputClass } from '@/lib/styles'

// maxReason is the server's bound on why a day is left out, in
// characters (day.MaxReasonLen).
const maxReason = 200

// Exclusion says whether calibration counts the day, and leaves it out
// with a reason (a restaurant meal that could not be weighed, being sick,
// a day not all logged) or counts it again. A reason a tap gives is
// stored under its code and said in the language; one typed, as it is.
export function Exclusion({ day }: { day: Day }) {
  const t = useMessages()
  const id = useId()
  const change = useCheckIn(day.date)
  // The form to give a reason, and the reason typed so far.
  const [open, setOpen] = useState(false)
  const [reason, setReason] = useState('')

  function send(excludedReason: string) {
    change.mutate(
      { params: { path: { date: day.date } }, body: { excluded_reason: excludedReason } },
      {
        onSuccess: () => {
          setOpen(false)
          setReason('')
        },
      },
    )
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    send(reasonOf(reason) ?? reason.trim())
  }

  // said is a reason as the language says it.
  const said = (r: string) => {
    const code = reasonOf(r)
    return code === undefined ? r : t.day.reasons[code]
  }

  let body
  if (day.status === 'excluded') {
    body = (
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm">{t.day.excluded(said(day.excluded_reason ?? ''))}</p>
        <Button
          variant="outline"
          disabled={change.isPending}
          onClick={() => {
            send('')
          }}
        >
          {t.day.countAgain}
        </Button>
      </div>
    )
  } else if (open) {
    body = (
      <form onSubmit={submit} className="space-y-2">
        <label htmlFor={id} className="sr-only">
          {t.day.reason}
        </label>
        <input
          id={id}
          value={reason}
          maxLength={maxReason}
          placeholder={t.day.reason}
          autoComplete="off"
          className={inputClass}
          onChange={(event) => {
            setReason(event.currentTarget.value)
          }}
        />
        <div className="flex flex-wrap gap-1.5">
          {Object.entries(t.day.reasons).map(([code, r]) => (
            <Button
              key={code}
              variant="outline"
              size="sm"
              className="rounded-full border-dashed"
              onClick={() => {
                setReason(r)
              }}
            >
              {r}
            </Button>
          ))}
        </div>
        <div className="flex gap-2">
          <Button type="submit" disabled={reason.trim() === '' || change.isPending}>
            {t.day.leaveOut}
          </Button>
          <Button
            variant="ghost"
            onClick={() => {
              setOpen(false)
            }}
          >
            {t.day.cancel}
          </Button>
        </div>
      </form>
    )
  } else {
    body = (
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t.day.counted}</p>
        <Button
          variant="outline"
          onClick={() => {
            setOpen(true)
          }}
        >
          {t.day.leaveOut}
        </Button>
      </div>
    )
  }

  return (
    <Field label={t.day.calibration} status={<SaveStatus change={change} />}>
      {body}
    </Field>
  )
}
