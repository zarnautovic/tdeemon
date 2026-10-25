import { Plus } from 'lucide-react'
import { useEffect, useId, useRef, useState, type SubmitEvent } from 'react'

import { detailOf, statusOf } from '@/api/errors'
import { useLogMeal } from '@/api/meals'
import type { Warning } from '@/api/types'
import { EntryRow } from '@/components/day/entry-row'
import { FoodPicker } from '@/components/day/food-picker'
import { estimateEntry, foodEntry, itemOf, most, type Entry } from '@/components/day/meal-entries'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'
import { uuidv7 } from '@/lib/uuid'
import { clockAt, mealInstant, nightAfter, toCome } from '@/lib/zoned'

interface Day {
  // The logical day, YYYY-MM-DD, and whether it is today.
  date: string
  today: boolean
  // The user's clock: its time zone and the hour a day starts at.
  timeZone: string
  startHour: number
}

// MealLog logs a meal of the day by hand: foods of the user's in their
// amounts, and estimates of whole plates, at a time of the day. Logging
// through a model stays the main way; this is for when there is none.
export function MealLog(day: Day) {
  const t = useMessages()
  const [open, setOpen] = useState(false)
  // What the server warned of the meal logged last.
  const [logged, setLogged] = useState<Warning[] | undefined>(undefined)
  // The button takes the focus back when the form closes.
  const button = useRef<HTMLButtonElement>(null)
  const [closed, setClosed] = useState(false)
  useEffect(() => {
    if (closed) {
      button.current?.focus()
    }
  }, [closed])

  if (open) {
    return (
      <Composer
        {...day}
        onDone={(warnings) => {
          setOpen(false)
          setClosed(true)
          setLogged(warnings)
        }}
        onCancel={() => {
          setOpen(false)
          setClosed(true)
        }}
      />
    )
  }
  return (
    <div className="space-y-1">
      <Button
        ref={button}
        variant="outline"
        size="lg"
        className="w-full"
        onClick={() => {
          setOpen(true)
          setClosed(false)
          setLogged(undefined)
        }}
      >
        <Plus aria-hidden />
        {t.log.open}
      </Button>
      {logged && (
        <output className="block text-sm">
          {t.log.logged}
          {logged.length > 0 && (
            <ul className="text-amber-700 dark:text-amber-400">
              {logged.map((w, i) => (
                <li key={`${w.code} ${String(i)}`}>{t.meal.warnings[w.code]}</li>
              ))}
            </ul>
          )}
        </output>
      )}
    </div>
  )
}

// Composer is a meal being logged. Its id is made when it opens, so the
// meal sent again (no answer came) is the same meal, not another: the
// form holds still until it is sent again, or given up, as other content
// under the id is a conflict.
function Composer({
  date,
  today,
  timeZone,
  startHour,
  onDone,
  onCancel,
}: Day & { onDone: (warnings: Warning[]) => void; onCancel: () => void }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const log = useLogMeal(date)
  const [mealId] = useState(uuidv7)
  const [description, setDescription] = useState('')
  // Now on today's, noon on another day's. Today's time left as it is
  // is the instant the meal is sent: the page may have been open a while.
  const [time, setTime] = useState(() => (today ? clockAt(Date.now(), timeZone) : '12:00'))
  const [timeSet, setTimeSet] = useState(false)
  // A time still to come, which the server refuses: a meal is logged once
  // eaten.
  const [later, setLater] = useState(false)
  const first = useRef<HTMLInputElement>(null)
  // The entry just added, whose amount takes the focus.
  const [added, setAdded] = useState<string | undefined>(undefined)
  useEffect(() => {
    first.current?.focus()
  }, [])
  const [entries, setEntries] = useState<Entry[]>([])
  // The entries that cannot be sent as they are, and whether there are
  // none to send.
  const [wrong, setWrong] = useState<ReadonlySet<string>>(new Set())
  const [empty, setEmpty] = useState(false)
  const status = statusOf(log.error)
  const refused = log.isError && (status === 409 || status === 422)
  const night = nightAfter(time, startHour)
  // Sent, and no answer said whether it was taken: the meal stays as sent.
  const held = log.isPending || (log.isError && !refused)

  function send(body: Parameters<typeof log.mutate>[0]['body']) {
    log.mutate(
      { body },
      {
        onSuccess: (answer) => {
          onDone(answer.warnings)
        },
      },
    )
  }

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    setEmpty(entries.length === 0)
    const bad = new Set<string>()
    const items = entries.flatMap((entry) => {
      const item = itemOf(entry, f.parse)
      if (item === undefined) {
        bad.add(entry.key)
        return []
      }
      return [item]
    })
    setWrong(bad)
    const eatenAt =
      today && !timeSet ? new Date().toISOString() : mealInstant(date, time, timeZone, startHour)
    setLater(toCome(eatenAt))
    if (entries.length === 0 || bad.size > 0 || toCome(eatenAt)) {
      return
    }
    const what = description.trim()
    send({
      id: mealId,
      ...(what ? { description: what } : {}),
      eaten_at: eatenAt,
      items,
    })
  }

  return (
    <form
      onSubmit={submit}
      noValidate
      aria-labelledby={`${id}-title`}
      className="space-y-4 rounded-xl border p-4"
    >
      <h3 id={`${id}-title`} className="font-medium">
        {t.log.title}
      </h3>
      <fieldset disabled={held} className="min-w-0 space-y-4">
        <div className="flex gap-3">
          <label className="flex-1 space-y-1">
            <span className={cn(fieldLabel, 'block')}>{t.log.description}</span>
            <input
              ref={first}
              value={description}
              maxLength={200}
              placeholder={t.log.descriptionHint}
              autoComplete="off"
              className={inputClass}
              onChange={(e) => {
                setDescription(e.currentTarget.value)
              }}
            />
          </label>
          <label className="w-28 space-y-1">
            <span className={cn(fieldLabel, 'block')}>{t.log.time}</span>
            <input
              type="time"
              value={time}
              required
              aria-invalid={later}
              aria-describedby={
                [later ? `${id}-later` : '', night ? `${id}-night` : ''].join(' ').trim() ||
                undefined
              }
              className={cn(inputClass, 'tabular-nums')}
              onChange={(e) => {
                // A cleared field keeps the time it had.
                const typed = e.currentTarget.value
                if (/^\d{2}:\d{2}$/.test(typed)) {
                  setTime(typed)
                  setTimeSet(true)
                  setLater(false)
                }
              }}
            />
          </label>
        </div>
        {night && (
          <p id={`${id}-night`} className="text-xs text-muted-foreground">
            {t.log.nightAfter(time)}
          </p>
        )}
        <section aria-label={t.log.items} className="space-y-2">
          {entries.length > 0 && (
            <ul className="divide-y">
              {entries.map((entry) => (
                <EntryRow
                  key={entry.key}
                  entry={entry}
                  wrong={wrong.has(entry.key)}
                  describedBy={wrong.has(entry.key) ? `${id}-wrong` : undefined}
                  focus={entry.key === added}
                  onChange={(changed) => {
                    setEntries((es) => es.map((e) => (e.key === entry.key ? changed : e)))
                  }}
                  onRemove={() => {
                    setEntries((es) => es.filter((e) => e.key !== entry.key))
                  }}
                />
              ))}
            </ul>
          )}
          <FoodPicker
            onPick={(food) => {
              const entry = foodEntry(food, f.field)
              setEntries((es) => [...es, entry])
              setAdded(entry.key)
              setEmpty(false)
            }}
          />
          <Button
            variant="outline"
            onClick={() => {
              const entry = estimateEntry()
              setEntries((es) => [...es, entry])
              setAdded(entry.key)
              setEmpty(false)
            }}
          >
            {t.log.estimate}
          </Button>
        </section>
      </fieldset>
      {later && (
        <p id={`${id}-later`} role="alert" className="text-sm text-destructive">
          {t.meal.timeToCome}
        </p>
      )}
      {empty && (
        <p role="alert" className="text-sm text-destructive">
          {t.log.noItems}
        </p>
      )}
      {entries.some((e) => wrong.has(e.key)) && (
        <p id={`${id}-wrong`} role="alert" className="text-sm text-destructive">
          {t.log.wrong(most(f.number))}
        </p>
      )}
      {refused ? (
        <p role="alert" className="text-sm text-destructive">
          {t.log.refused(detailOf(log.error))}
        </p>
      ) : (
        <SaveStatus
          change={log}
          // Again, as it was sent, under its id: the server takes the same
          // meal sent again for the one it has.
          onRetry={() => {
            if (log.variables) {
              send(log.variables.body)
            }
          }}
        />
      )}
      <div className="flex gap-2">
        <Button type="submit" size="lg" disabled={held}>
          {t.log.save}
        </Button>
        <Button variant="ghost" size="lg" disabled={log.isPending} onClick={onCancel}>
          {t.log.cancel}
        </Button>
      </div>
    </form>
  )
}
