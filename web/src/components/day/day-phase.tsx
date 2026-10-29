import { Link } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { daysBetween, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

type Verdict = components['schemas']['VerdictBody']

// The dot of a phase's line: the brand's green on target, the warning's
// amber off it; the words beside it say which.
const dots: Record<Verdict['rate'], string> = {
  none: 'bg-muted-foreground',
  on_target: 'bg-success',
  below: 'bg-warning',
  above: 'bg-warning',
}

// DayPhase is the phase of the plan a day is in, and its week of the
// phase's; of the phase now, the rate calibration measures and where it
// stands against the phase's: a line that leads to the progress, in the
// day's hero on a phone. A desktop has it as a card of its own (card),
// beside the day, with how far into the phase the day is, when the phase
// has an end. Nothing without a phase.
export function DayPhase({ date, card = false }: { date: string; card?: boolean }) {
  const t = useMessages()
  const f = useFormat()
  const plan = $api.useQuery('get', '/api/v1/plan')
  const summary = $api.useQuery('get', '/api/v1/summary')
  const verdict = summary.data?.verdict
  const today = plan.data?.today
  const phase = plan.data?.phases.find(
    (p) => p.start <= date && (p.end === undefined || date <= p.end),
  )
  if (phase === undefined || today === undefined) {
    return null
  }
  const week = Math.floor(daysBetween(phase.start, date) / 7) + 1
  const weeks =
    phase.end === undefined ? undefined : Math.ceil((daysBetween(phase.start, phase.end) + 1) / 7)
  // The verdict is of the phase today is in.
  const now = phase.start <= today && (phase.end === undefined || today <= phase.end)
  const rate = now && verdict?.measured_pct_week !== undefined ? verdict.rate : 'none'

  const line = (
    <Link
      to="/progress"
      className="flex items-center gap-2 rounded-md bg-surface-2 px-3 py-2.5 text-small outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
    >
      <span aria-hidden className={cn('size-2 shrink-0 rounded-full', dots[rate])} />
      <span className="flex-1">
        {t.plan.change.types[phase.type]} ·{' '}
        {weeks === undefined ? t.day.summary.week(week) : t.day.summary.weekOf(week, weeks)}
        {rate !== 'none' && verdict?.measured_pct_week !== undefined && (
          <>
            {' · '}
            <b className="font-semibold">
              {t.day.summary.rate(f.signed(verdict.measured_pct_week, 1))}
            </b>
            , {t.day.summary.rates[rate]}
          </>
        )}
      </span>
      <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
    </Link>
  )
  if (!card) {
    return line
  }
  const p = t.plan.phase
  // Its days up to the day, the day's included, of all of them.
  const total = phase.end === undefined ? undefined : daysBetween(phase.start, phase.end) + 1
  const gone =
    total === undefined
      ? undefined
      : Math.min(Math.max(daysBetween(phase.start, date) + 1, 0), total)

  return (
    <section aria-labelledby="day-phase" className="space-y-3 rounded-lg border bg-card p-4">
      <h2 id="day-phase" className="text-h3">
        {t.day.phase}
      </h2>
      {line}
      {total !== undefined && gone !== undefined && phase.end !== undefined && (
        <div className="space-y-1.5">
          <progress
            aria-label={p.progress}
            aria-valuetext={p.left(total - gone)}
            max={total}
            value={gone}
            className="block h-2 w-full appearance-none overflow-hidden rounded-full bg-surface-2 [&::-moz-progress-bar]:rounded-full [&::-moz-progress-bar]:bg-primary [&::-webkit-progress-bar]:bg-surface-2 [&::-webkit-progress-value]:rounded-full [&::-webkit-progress-value]:bg-primary"
          />
          <p className="flex justify-between gap-2 text-caption text-muted-foreground">
            <span>{p.span(f.short(phase.start), f.short(phase.end))}</span>
            <span>{p.left(total - gone)}</span>
          </p>
        </div>
      )}
    </section>
  )
}
