import { useLocation, useSearch } from '@tanstack/react-router'
import { useId, useRef, useState, type RefObject } from 'react'
import { flushSync } from 'react-dom'

import { detailOf, statusOf } from '@/api/errors'
import { useLogMeal } from '@/api/meals'
import type { components } from '@/api/schema'
import type { Food, Meal } from '@/api/types'
import { Finder } from '@/components/day/finder'
import {
  entryValues,
  foodEntry,
  itemOf,
  repeatOf,
  servingOf,
  servingsOf,
  type Entry,
} from '@/components/day/meal-entries'
import { mealName, namedBy, slotAt } from '@/components/day/meal-slot'
import { NewEntry, type Making } from '@/components/day/new-entry'
import { SaveStatus } from '@/components/day/save-status'
import { WeighInChip } from '@/components/day/weigh-in-chip'
import { Sheet } from '@/components/sheet'
import { Button } from '@/components/ui/button'
import { TimeInput } from '@/components/ui/time-input'
import { rounded, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { touchScreen } from '@/lib/media'
import { fieldLabel, inputClass, linkTarget } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'
import { uuidv7 } from '@/lib/uuid'
import { clockAt, mealInstant, nightAfter, parseClock, toCome } from '@/lib/zoned'

type LogMealBody = components['schemas']['LogMealBody']

interface Day {
  // The logical day, YYYY-MM-DD, and whether it is today.
  date: string
  today: boolean
  // The user's clock: its time zone and the hour a day starts at.
  timeZone: string
  startHour: number
}

interface Start {
  // What the day's target leaves, when it has one.
  remaining: number | undefined
  // A food the meal starts with.
  initial?: Food | undefined
  onLogged?: ((meal: Meal) => void) | undefined
}

// QuickAdd logs a meal of the day by hand, in a sheet: the foods of the
// user's that a search finds, or that were eaten lately or most, each in
// the amount it was eaten in last or another, and estimates, gathered in
// the meal at the sheet's foot with what each brings, the total, and what
// the day has left after it; or a meal of before, repeated in a tap,
// those eaten lately at the meal of the day first. A new food or an
// estimate is a step of its own, in the search's place. The meal is
// timed now (on another day, at noon), and named by what is in it, both
// of which can be changed. Each opening is a meal anew. The focus goes to
// the search, but on a touch screen, whose keyboard would cover the
// lists, until it is tapped. opener is what opened it, which the focus
// goes back to; onLogged is told the meal logged, before the sheet
// closes, so the focus may go to the meal's row instead.
export function QuickAdd({
  open,
  onOpenChange,
  opener,
  ...meal
}: Day &
  Start & {
    open: boolean
    onOpenChange: (open: boolean) => void
    opener?: RefObject<HTMLElement | null>
  }) {
  const t = useMessages()
  const search = useRef<HTMLInputElement>(null)
  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title={t.log.title}
      initialFocus={search}
      tapToType
      finalFocus={opener}
      tall
      bare
    >
      <Composer
        {...meal}
        search={search}
        onDone={() => {
          onOpenChange(false)
        }}
      />
    </Sheet>
  )
}

// Composer is a meal being logged. Its id is made as it is sent, so the
// meal sent again (no answer came) is the same meal, not another: it
// holds still until it is sent again, or given up. Logged, it is done,
// and a toast says so, with what the server warned of it and Undo, and,
// over another page than its day's, the way to the day. Its
// next step (a new food, an estimate) takes the search's place, which
// stays as it was under it, and the foot's: one button, Add to meal.
function Composer({
  remaining,
  initial,
  onLogged,
  search,
  onDone,
  date,
  today,
  timeZone,
  startHour,
}: Day &
  Start & {
    search: RefObject<HTMLInputElement | null>
    onDone: () => void
  }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  // Whether the meal sent is one of before, logged again, which its toast
  // says.
  const repeating = useRef(false)
  // Whether the page is the meal's day's: today's is / itself.
  const onDay = useLocation({ select: (l) => l.pathname === '/' })
  const shown = useSearch({ strict: false, select: (s) => s.date })
  const dayShown = onDay && (shown === date || (today && shown === undefined))
  const log = useLogMeal(date, (answer) => {
    const said = repeating.current ? t.meal.repeated : t.toast.logged
    sayChange(
      {
        title: said(mealName(answer.meal, t), t.unit.kcal(f.number(answer.meal.total.kcal))),
        lines: answer.warnings.map((w) => t.meal.warnings[w.code]),
        to: dayShown ? undefined : date,
      },
      { last: { action: 'meal.create', entity: answer.meal.id }, date },
    )
    onLogged?.(answer.meal)
  })
  const [entries, setEntries] = useState<Entry[]>(() =>
    initial === undefined ? [] : [foodEntry(initial, f.field)],
  )
  const [details, setDetails] = useState(false)
  const [description, setDescription] = useState('')
  // Now on today's, noon on another day's. Today's time left as it is
  // is the instant the meal is sent (eatenAt): the sheet may have been
  // open a while.
  const [time, setTime] = useState(() => (today ? clockAt(Date.now(), timeZone) : '12:00'))
  const [timeSet, setTimeSet] = useState(false)
  // The time as typed, on the language's clock, and whether it is none
  // (parseClock).
  const [timeText, setTimeText] = useState(() => f.clock(time))
  const [badTime, setBadTime] = useState(false)
  // A time still to come, which the server refuses: a meal is logged once
  // eaten.
  const [later, setLater] = useState(false)
  const status = statusOf(log.error)
  const refused = log.isError && (status === 409 || status === 422)
  // Sent, and no answer said whether it was taken: the meal stays as sent.
  const held = log.isPending || (log.isError && !refused)
  const night = nightAfter(time, startHour)
  const at = mealInstant(date, time, timeZone, startHour)
  const slotOf = slotAt(Date.parse(at), timeZone)
  const slot = t.meal.slots[slotOf]
  // When the sheet opened: what was eaten in the two weeks before counts
  // for what it offers first.
  const [now] = useState(Date.now)
  const [query, setQuery] = useState('')
  const [making, setMaking] = useState<Making | undefined>(undefined)
  // The button that opened the next step, which the focus goes back to.
  const maker = useRef<HTMLElement | null>(null)
  const logButton = useRef<HTMLButtonElement>(null)

  const values = entries.map((e) => entryValues(e, f.parse))
  const kcal = values.reduce((sum, v) => sum + (v?.kcal ?? 0), 0)
  // What the day has left after the meal: what it leaves less the meal's
  // kcal as they are shown, so the two add up.
  const after = remaining === undefined ? undefined : remaining - rounded(kcal)

  function send(body: LogMealBody) {
    log.mutate({ body }, { onSuccess: onDone })
  }

  // eatenAt is when a meal is sent as eaten: today's time left as it is,
  // the server's when it takes it, from the meal's id, made as it is sent;
  // or undefined when the time is still to come, which is said.
  function eatenAt(): { eaten_at?: string } | undefined {
    const typed = parseClock(timeText)
    setBadTime(typed === undefined)
    if (typed === undefined) {
      return undefined
    }
    const ahead = !(today && !timeSet) && toCome(at)
    setLater(ahead)
    if (ahead) {
      return undefined
    }
    return today && !timeSet ? {} : { eaten_at: at }
  }

  function submit() {
    const when = eatenAt()
    // Each entry was checked as it came: a food's amount, an estimate's
    // values.
    const items = entries.flatMap((e) => {
      const item = itemOf(e, f.parse)
      return item === undefined ? [] : [item]
    })
    if (items.length === 0 || when === undefined) {
      return
    }
    const what = description.trim()
    repeating.current = false
    send({ id: uuidv7(), ...(what ? { description: what } : {}), ...when, items })
  }

  function repeat(meal: Meal) {
    const when = eatenAt()
    if (when !== undefined) {
      repeating.current = true
      send({ ...repeatOf(meal), ...when })
    }
  }

  // added takes an entry into the meal, from the lists or the next step,
  // and readies the search for the next: emptied, with the focus; on a
  // touch screen, whose keyboard would cover the lists, the meal's button
  // has it.
  function added(entry: Entry) {
    flushSync(() => {
      setEntries((es) => [...es, entry])
      setQuery('')
      setMaking(undefined)
    })
    const next = touchScreen() ? logButton.current : search.current
    next?.focus()
  }

  // back goes back to the search from the next step, to the button that
  // opened it.
  function back() {
    flushSync(() => {
      setMaking(undefined)
    })
    maker.current?.focus()
  }

  // amountOf is how much of a food an entry has, as its field says it (a
  // serving's, servingsOf).
  function amountOf(entry: Entry & { kind: 'food' }): string {
    if (entry.unit === 'portion') {
      return `${entry.amount} ${t.log.portions(f.parse(entry.amount) ?? Number.NaN)}`
    }
    const serving = servingOf(entry.unit)
    return serving === undefined
      ? `${entry.amount} ${entry.unit}`
      : servingsOf(entry.amount, f.parse(entry.amount) ?? Number.NaN, serving)
  }

  return (
    <>
      {making !== undefined && (
        <NewEntry making={making} name={query.trim()} onBack={back} onAdd={added} />
      )}
      {/* Under the next step, as it was. */}
      <div hidden={making !== undefined} className="contents">
        <div className="flex flex-wrap items-baseline gap-x-2 px-4 pt-2 text-small text-muted-foreground">
          <span>{t.log.when(slot, today ? t.log.today : f.short(date), f.time(at, timeZone))}</span>
          <button
            type="button"
            aria-expanded={details}
            aria-controls={`${id}-details`}
            aria-label={t.log.changeOf}
            disabled={held}
            className={cn(
              linkTarget,
              'font-semibold text-primary underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring',
            )}
            onClick={() => {
              setDetails((d) => !d)
            }}
          >
            {t.log.change}
          </button>
        </div>
        <fieldset disabled={held} className="min-h-0 min-w-0 flex-1 space-y-3 overflow-y-auto p-4">
          {details && (
            <div id={`${id}-details`} className="space-y-1">
              <div className="flex gap-3">
                <label className="flex-1 space-y-1">
                  <span className={cn(fieldLabel, 'block')}>{t.log.description}</span>
                  <input
                    value={description}
                    maxLength={200}
                    // The name the meal gets without one: what is in it.
                    placeholder={namedBy(
                      entries.map((e) => (e.kind === 'food' ? e.food.name : e.name.trim())),
                      t,
                    )}
                    autoComplete="off"
                    className={inputClass}
                    onChange={(e) => {
                      setDescription(e.currentTarget.value)
                    }}
                  />
                </label>
                <label className="w-28 space-y-1">
                  <span className={cn(fieldLabel, 'block')}>{t.log.time}</span>
                  <TimeInput
                    value={timeText}
                    aria-invalid={later || badTime}
                    aria-describedby={night ? `${id}-night` : undefined}
                    onChange={(e) => {
                      const typed = e.currentTarget.value
                      setTimeText(typed)
                      setBadTime(false)
                      setLater(false)
                      const clock = parseClock(typed)
                      if (clock !== undefined) {
                        setTime(clock)
                        setTimeSet(true)
                      }
                    }}
                    onBlur={() => {
                      // As the time it reads, on the language's clock:
                      // 7:30 PM, or 19:30, for 1930.
                      setTimeText((typed) => {
                        const clock = parseClock(typed)
                        return clock === undefined ? typed : f.clock(clock)
                      })
                    }}
                  />
                </label>
              </div>
              {night && (
                <p id={`${id}-night`} className="text-xs text-muted-foreground">
                  {t.log.nightAfter(f.clock(time))}
                </p>
              )}
            </div>
          )}
          <Finder
            search={search}
            query={query}
            onQuery={setQuery}
            at={{ slot: slotOf, now, timeZone, date }}
            onAdd={added}
            onRepeat={repeat}
            onMake={(m, button) => {
              maker.current = button
              setMaking(m)
            }}
          />
          <WeighInChip date={date} />
        </fieldset>
        <div className="space-y-2 border-t p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {entries.length > 0 ? (
            <ul aria-label={t.log.basket} className="flex flex-wrap gap-1.5">
              {entries.map((entry, i) => {
                const name = entry.kind === 'food' ? entry.food.name : entry.name
                const v = values[i]
                return (
                  <li
                    key={entry.key}
                    className="flex items-center gap-0.5 rounded-full bg-surface-2 pl-3 text-small"
                  >
                    <span>
                      {[
                        name,
                        ...(entry.kind === 'food' ? [amountOf(entry)] : []),
                        ...(v === undefined ? [] : [t.unit.kcal(f.number(v.kcal))]),
                      ].join(' · ')}
                    </span>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="rounded-full"
                      aria-label={t.log.remove(name)}
                      disabled={held}
                      onClick={() => {
                        setEntries((es) => es.filter((e) => e.key !== entry.key))
                      }}
                    >
                      <span aria-hidden>×</span>
                    </Button>
                  </li>
                )
              })}
            </ul>
          ) : (
            <p id={`${id}-empty`} className="text-small text-muted-foreground">
              {t.log.empty}
            </p>
          )}
          {after !== undefined && entries.length > 0 && (
            <p className="text-small text-muted-foreground">
              {after >= 0
                ? t.log.after(f.number(Math.round(after)))
                : t.log.afterOver(f.number(Math.round(-after)))}
            </p>
          )}
          {(later || badTime) && (
            <p role="alert" className="text-sm text-destructive">
              {badTime ? t.meal.timeFormat(f.clock('19:30')) : t.meal.timeToCome}
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
          {/* Disabled, it stays in reach of the focus, and says what it waits for. */}
          <Button
            ref={logButton}
            className="h-13 w-full text-base"
            disabled={held || entries.length === 0}
            focusableWhenDisabled
            aria-describedby={entries.length === 0 ? `${id}-empty` : undefined}
            onClick={submit}
          >
            {entries.length > 0
              ? t.log.logItems(entries.length, f.number(Math.round(kcal)))
              : t.log.save}
          </Button>
        </div>
      </div>
    </>
  )
}
