import { useId } from 'react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { EditPhase } from '@/components/plan/edit-phase'
import { PhaseTimeline } from '@/components/plan/phase-timeline'
import { spansOf, type Span } from '@/components/plan/phases'
import { useDate } from '@/components/plan/proposal-text'
import { daysBetween, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

type Plan = components['schemas']['PlanBody']
type Verdict = components['schemas']['VerdictBody']

// The dot beside the rate measured: the brand's in range, the warning's
// out of it; the words beside it say which.
const dots: Record<Verdict['rate'], string> = {
  none: 'bg-muted-foreground',
  on_target: 'bg-success',
  below: 'bg-warning',
  above: 'bg-warning',
}

// PhaseCard is the plan's hero: the phase today is in, its type, its
// target rate with the range calibration holds it to and what that is in
// kg, the rate measured, its week, how far it has come to its planned end
// and to its goal; then the phases on a timeline, past, today's and to
// come. Without a phase today, the next one. The user edits the phase it
// shows, and plans the next.
export function PhaseCard({ plan }: { plan: Plan }) {
  const t = useMessages()
  const date = useDate()
  const id = useId()
  const p = t.plan.phase
  const spans = spansOf(plan.phases, plan.today)
  const now = spans.find((s) => s.state === 'now')
  const next = spans.find((s) => s.state === 'planned')
  // The phase the card shows, which the user edits.
  const shown = now ?? next

  return (
    <section
      aria-labelledby={id}
      className="space-y-4 rounded-xl border bg-card p-4 shadow-card lg:p-6"
    >
      {now === undefined ? (
        <header className="space-y-1">
          <h2 id={id} className="text-h2">
            {p.none}
          </h2>
          {next !== undefined && (
            <p className="text-small text-muted-foreground">
              {p.next(t.plan.change.types[next.phase.type], date(next.phase.start))}
            </p>
          )}
        </header>
      ) : (
        <Now span={now} today={plan.today} titleId={id} />
      )}
      {(now === undefined || spans.length > 1) && <PhaseTimeline spans={spans} />}
      <div className="flex flex-wrap gap-2">
        {shown !== undefined && <EditPhase plan={plan} phase={shown.phase} />}
        <EditPhase plan={plan} />
      </div>
    </section>
  )
}

// Now is the phase of today, by its numbers.
function Now({ span, today, titleId }: { span: Span; today: string; titleId: string }) {
  const t = useMessages()
  const f = useFormat()
  const date = useDate()
  const summary = $api.useQuery('get', '/api/v1/summary')
  const p = t.plan.phase
  const { phase, last } = span
  const week = Math.floor(daysBetween(phase.start, today) / 7) + 1
  const weeks = last === undefined ? undefined : Math.ceil((daysBetween(phase.start, last) + 1) / 7)
  const trend = summary.data?.trend?.kg
  const verdict = summary.data?.verdict
  // The verdict is of the phase today is in, and holds a rate measured
  // only when calibration trusts one.
  const measured = verdict?.rate !== 'none' ? verdict?.measured_pct_week : undefined
  const tolerance = verdict?.tolerance_pct_week
  const rate = phase.rate_pct_week
  const pct = (n: number) => f.percent(n, 2)

  return (
    <>
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-small text-muted-foreground">{p.current(date(phase.start))}</p>
          <h2 id={titleId} className="text-title">
            {t.plan.change.types[phase.type]}
          </h2>
        </div>
        <span className="shrink-0 rounded-full bg-primary-soft px-3 py-1 text-small text-primary">
          {weeks === undefined ? p.week(week) : p.weekOf(Math.min(week, weeks), weeks)}
        </span>
      </header>
      <dl className="grid grid-cols-2 gap-3">
        <div className="space-y-0.5 rounded-md bg-surface-2 p-3">
          <dt className="text-small text-muted-foreground">{p.pace}</dt>
          <dd className="text-h3">{t.plan.change.rate(pct(rate))}</dd>
          {trend !== undefined && rate !== 0 && (
            <dd className="text-caption text-muted-foreground">
              {p.kg(f.signed((rate / 100) * trend, 2))}
            </dd>
          )}
          {tolerance !== undefined && (
            <dd className="text-caption text-muted-foreground">
              {p.band(pct(rate - tolerance), pct(rate + tolerance))}
            </dd>
          )}
        </div>
        <div className="space-y-0.5 rounded-md bg-surface-2 p-3">
          <dt className="text-small text-muted-foreground">{p.measured}</dt>
          {measured === undefined || verdict === undefined ? (
            <dd className="text-small text-muted-foreground">{p.notMeasured}</dd>
          ) : (
            <>
              <dd className="text-h3">{t.plan.change.rate(pct(measured))}</dd>
              <dd className="flex items-center gap-1.5 text-caption text-muted-foreground">
                <span aria-hidden className={cn('size-2 rounded-full', dots[verdict.rate])} />
                {p.rates[verdict.rate]}
              </dd>
            </>
          )}
        </div>
      </dl>
      {last !== undefined && <Progress start={phase.start} last={last} today={today} />}
      {/* A goal, and a milestone before it; or a milestone alone. */}
      {phase.goal_weight === undefined && phase.milestone_weight !== undefined && (
        <p className="text-small">{p.milestoneAlone(f.number(phase.milestone_weight, 1))}</p>
      )}
      {phase.goal_weight !== undefined && (
        <p className="text-small">
          {p.goal(f.number(phase.goal_weight, 1))}
          {phase.milestone_weight !== undefined && (
            <span className="text-muted-foreground">
              {' · '}
              {p.milestone(f.number(phase.milestone_weight, 1))}
            </span>
          )}
          {trend !== undefined && (
            <span className="text-muted-foreground">
              {' · '}
              {p.trend(f.number(trend, 1), f.number(Math.abs(trend - phase.goal_weight), 1))}
            </span>
          )}
        </p>
      )}
    </>
  )
}

// Progress is how far a phase with a planned end has come: its days gone
// by, today's included, of all of them, as a bar, and the days left.
function Progress({ start, last, today }: { start: string; last: string; today: string }) {
  const t = useMessages()
  const date = useDate()
  const p = t.plan.phase
  const total = daysBetween(start, last) + 1
  const gone = Math.min(Math.max(daysBetween(start, today) + 1, 0), total)
  const left = total - gone

  return (
    <div className="space-y-1.5">
      <progress
        aria-label={p.progress}
        aria-valuetext={p.left(left)}
        max={total}
        value={gone}
        className="block h-2 w-full appearance-none overflow-hidden rounded-full bg-surface-2 [&::-moz-progress-bar]:rounded-full [&::-moz-progress-bar]:bg-primary [&::-webkit-progress-bar]:bg-surface-2 [&::-webkit-progress-value]:rounded-full [&::-webkit-progress-value]:bg-primary"
      />
      <p className="flex justify-between gap-2 text-caption text-muted-foreground">
        <span>{p.span(date(start), date(last))}</span>
        <span>{p.left(left)}</span>
      </p>
    </div>
  )
}
