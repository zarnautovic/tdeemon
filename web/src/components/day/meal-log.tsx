import { Plus } from 'lucide-react'
import { useId, useRef, useState, type SubmitEvent } from 'react'

import { detailOf, statusOf } from '@/api/errors'
import { useLogMeal } from '@/api/meals'
import type { Warning } from '@/api/types'
import { EntryRow } from '@/components/day/entry-row'
import { FoodPicker } from '@/components/day/food-picker'
import { estimateEntry, foodEntry, itemOf, type Entry } from '@/components/day/meal-entries'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'
import { uuidv7 } from '@/lib/uuid'
import { clockAt, mealInstant } from '@/lib/zoned'

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

  if (open) {
    return (
      <Composer
        {...day}
        onDone={(warnings) => {
          setOpen(false)
          setLogged(warnings)
        }}
        onCancel={() => {
          setOpen(false)
        }}
      />
    )
  }
  return (
    <div className="space-y-1">
      <Button
        variant="outline"
        size="lg"
        className="w-full"
        onClick={() => {
          setOpen(true)
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
// meal sent again (no answer came) is the same meal, not another.
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
  const form = useRef<HTMLFormElement>(null)
  const [mealId] = useState(uuidv7)
  const [description, setDescription] = useState('')
  // Now on today's, noon on another day's.
  const [time, setTime] = useState(() => (today ? clockAt(Date.now(), timeZone) : '12:00'))
  const [entries, setEntries] = useState<Entry[]>([])
  // The entries that cannot be sent as they are, and whether there are
  // none to send.
  const [wrong, setWrong] = useState<ReadonlySet<string>>(new Set())
  const [empty, setEmpty] = useState(false)
  const status = statusOf(log.error)
  const refused = log.isError && (status === 409 || status === 422)

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
    if (entries.length === 0 || bad.size > 0) {
      return
    }
    const what = description.trim()
    log.mutate(
      {
        body: {
          id: mealId,
          ...(what ? { description: what } : {}),
          eaten_at: mealInstant(date, time, timeZone, startHour),
          items,
        },
      },
      {
        onSuccess: (answer) => {
          onDone(answer.warnings)
        },
      },
    )
  }

  return (
    <form
      ref={form}
      onSubmit={submit}
      noValidate
      aria-labelledby={`${id}-title`}
      className="space-y-4 rounded-xl border p-4"
    >
      <h3 id={`${id}-title`} className="font-medium">
        {t.log.title}
      </h3>
      <div className="flex gap-3">
        <label className="flex-1 space-y-1">
          <span className={cn(fieldLabel, 'block')}>{t.log.description}</span>
          <input
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
            className={cn(inputClass, 'tabular-nums')}
            onChange={(e) => {
              // A cleared field keeps the time it had.
              const typed = e.currentTarget.value
              if (/^\d{2}:\d{2}$/.test(typed)) {
                setTime(typed)
              }
            }}
          />
        </label>
      </div>
      <section aria-label={t.log.items} className="space-y-2">
        {entries.length > 0 && (
          <ul className="divide-y">
            {entries.map((entry) => (
              <EntryRow
                key={entry.key}
                entry={entry}
                wrong={wrong.has(entry.key)}
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
            setEntries((es) => [...es, foodEntry(food, f.field)])
            setEmpty(false)
          }}
        />
        <Button
          variant="outline"
          onClick={() => {
            setEntries((es) => [...es, estimateEntry()])
            setEmpty(false)
          }}
        >
          {t.log.estimate}
        </Button>
      </section>
      {empty && (
        <p role="alert" className="text-sm text-destructive">
          {t.log.noItems}
        </p>
      )}
      {wrong.size > 0 && (
        <p role="alert" className="text-sm text-destructive">
          {t.day.notANumber}
        </p>
      )}
      {refused ? (
        <p role="alert" className="text-sm text-destructive">
          {t.log.refused(detailOf(log.error))}
        </p>
      ) : (
        <SaveStatus
          change={log}
          // Again, as the meal is now, under its id: the server takes the
          // same meal sent again for the one it has.
          onRetry={() => {
            form.current?.requestSubmit()
          }}
        />
      )}
      <div className="flex gap-2">
        <Button type="submit" size="lg" disabled={log.isPending}>
          {t.log.save}
        </Button>
        <Button variant="ghost" size="lg" onClick={onCancel}>
          {t.log.cancel}
        </Button>
      </div>
    </form>
  )
}
