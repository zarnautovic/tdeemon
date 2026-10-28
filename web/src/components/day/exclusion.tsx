import { Switch } from '@base-ui/react/switch'
import { useId, useState, type SubmitEvent } from 'react'

import { useCheckIn } from '@/api/day'
import type { Day } from '@/api/types'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { reasonOf, useMessages } from '@/i18n/messages'
import { inputClass } from '@/lib/styles'
import { sayChange } from '@/lib/toast'

// maxReason is the server's bound on why a day is left out, in
// characters (day.MaxReasonLen).
const maxReason = 200

// Exclusion says whether calibration counts the day, as a switch: off,
// it leaves the day out with a reason (a restaurant meal that could not
// be weighed, being sick, a day not all logged); on, it counts it again.
// A reason a tap gives is stored under its code and said in the
// language; one typed, as it is.
export function Exclusion({ day }: { day: Day }) {
  const t = useMessages()
  const id = useId()
  const change = useCheckIn(day.date, (answer) => {
    sayChange(
      { title: answer.status === 'excluded' ? t.toast.excluded : t.toast.included },
      { last: { action: 'day.checkin', entity: day.date }, date: day.date },
    )
  })
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

  const excluded = day.status === 'excluded'

  return (
    <div className="space-y-2">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 space-y-0.5">
          <span id={`${id}-label`} className="block text-sm font-medium">
            {t.day.inCalibration}
          </span>
          <p className="text-small text-muted-foreground">
            {excluded ? t.day.excluded(said(day.excluded_reason ?? '')) : t.day.calibrationHint}
          </p>
        </div>
        {/* On, the day counts; off, a reason is asked, and the day left
            out with it. */}
        <Switch.Root
          aria-labelledby={`${id}-label`}
          checked={!excluded && !open}
          disabled={change.isPending}
          onCheckedChange={(on) => {
            if (on && excluded) {
              send('')
            } else {
              setOpen(!on)
            }
          }}
          className="relative mt-0.5 inline-flex h-7 w-12 shrink-0 items-center rounded-full bg-surface-2 p-0.5 outline-none after:absolute after:-inset-2 after:content-[''] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card data-checked:bg-primary data-disabled:opacity-45"
        >
          <Switch.Thumb className="size-6 rounded-full bg-card shadow-card transition-transform duration-(--duration-base) data-checked:translate-x-5" />
        </Switch.Root>
      </div>
      {open && !excluded && (
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
      )}
      <SaveStatus change={change} />
    </div>
  )
}
