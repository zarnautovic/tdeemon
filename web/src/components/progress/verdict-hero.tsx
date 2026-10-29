import { CircleAlert, CircleCheck, Flag, Hourglass, type LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { LoadError } from '@/components/load-error'
import { CalibrationCard } from '@/components/progress/calibration-card'
import { PlanRow } from '@/components/progress/plan-row'
import { phaseSaid, recoverySaid } from '@/components/progress/said'
import { Skeleton } from '@/components/progress/skeleton'
import { card, entry } from '@/components/progress/styles'
import { Badge } from '@/components/ui/badge'
import { InfoTip } from '@/components/ui/info-tip'
import type { Messages } from '@/i18n/en'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

type Verdict = components['schemas']['VerdictBody']
type Phase = components['schemas']['PhaseBody']
type Said = Messages['charts']['verdict']

// State is what the hero says of the phase: on track, faster or slower
// than its range, not enough data yet for a verdict, or no phase or
// target to judge by.
type State = 'on_target' | 'fast' | 'slow' | 'waiting' | 'no_phase' | 'no_target'

// stateOf is the verdict's state. Below the range is a faster loss or a
// slower gain than planned, above it the other way; a phase that keeps
// the weight (a rate of 0) moves too fast either way.
function stateOf(v: Verdict): State {
  if (v.rate !== 'none' && v.target_pct_week !== undefined) {
    if (v.rate === 'on_target') {
      return 'on_target'
    }
    if (v.target_pct_week === 0) {
      return 'fast'
    }
    return (v.rate === 'below') === v.target_pct_week < 0 ? 'fast' : 'slow'
  }
  if (v.reasons.includes('no_phase')) {
    return 'no_phase'
  }
  return v.reasons.includes('no_target') ? 'no_target' : 'waiting'
}

// The hero's look in each state: on track the brand's, off the range the
// warning's, else plain; the headline and its icon say which, never the
// colour alone.
const looks: Record<State, { icon: LucideIcon; card: string; tint: string }> = {
  on_target: { icon: CircleCheck, card: 'border-primary/30 bg-primary-soft', tint: 'text-primary' },
  fast: { icon: CircleAlert, card: 'border-warning/30 bg-warning/10', tint: 'text-warning' },
  slow: { icon: CircleAlert, card: 'border-warning/30 bg-warning/10', tint: 'text-warning' },
  waiting: { icon: Hourglass, card: 'bg-card', tint: 'text-muted-foreground' },
  no_phase: { icon: Flag, card: 'bg-card', tint: 'text-muted-foreground' },
  no_target: { icon: Flag, card: 'bg-card', tint: 'text-muted-foreground' },
}

// VerdictHero is the page's one fact that matters most: how the phase
// goes, as calibration judges it (GET /api/v1/summary). On track, too
// fast or too slow, it says the rate measured against the phase's range;
// without a verdict yet, why. Beside it, how far the TDEE is to be
// trusted, the trend weight, what was eaten against the target, and how
// the body recovered. Until the TDEE is reliable, calibration's card
// stands in its place; without a phase or a target, the TDEE leads, and
// the way to the plan follows. The verdict, the TDEE, its trust and the
// trend weight are each an ⓘ away from what they are.
export function VerdictHero({ phase, today }: { phase: Phase | undefined; today: string }) {
  const t = useMessages()
  const f = useFormat()
  const summary = $api.useQuery('get', '/api/v1/summary')
  const v = t.charts.verdict
  const i = t.info

  if (summary.isError) {
    return (
      <section aria-label={v.label} className={card}>
        <LoadError error={summary.error} onRetry={() => void summary.refetch()} />
      </section>
    )
  }
  if (!summary.data) {
    return (
      <section aria-label={v.label} aria-busy className={cn(card, 'space-y-3 lg:p-6')}>
        <output className="sr-only">{t.loading}</output>
        <Skeleton className="h-4 w-40" />
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-5 w-full max-w-md" />
        <Skeleton className="h-5 w-64" />
      </section>
    )
  }

  const { verdict, tdee, trend, recovery } = summary.data
  const state = stateOf(verdict)
  const missing = state === 'no_phase' || state === 'no_target' ? state : undefined
  // The phase and its week, over the headline.
  const ofPhase =
    phase !== undefined && state !== 'no_phase' ? phaseSaid(phase, today, t) : undefined
  if (!tdee.trusted) {
    return <CalibrationCard summary={summary.data} phase={ofPhase} missing={missing} />
  }

  const look = looks[state]
  const Icon = look.icon
  const headlines: Record<State, string> = {
    on_target: v.onTarget,
    fast: v.fast,
    slow: v.slow,
    waiting: v.waiting,
    no_phase: v.noPhase,
    no_target: v.noTarget,
  }

  let said: string | undefined
  if (state === 'waiting') {
    said =
      verdict.reasons.includes('before_phase') && tdee.before_phase_until !== undefined
        ? t.calibration.beforePhase(f.short(tdee.before_phase_until))
        : v.reasons[verdict.reasons.includes('no_tdee') ? 'no_tdee' : 'before_phase']
  } else if (missing === undefined) {
    said = rateSaid(verdict, state === 'on_target', v, f)
  }
  const recovered = recoverySaid(recovery, t, f)

  // The TDEE and how far it is to be trusted: the headline without a
  // phase or a target to judge by, else beside the verdict.
  const trust = tdee.before_phase ? 'before' : 'trusted'
  const measured = tdee.kcal !== undefined && tdee.half_width !== undefined && (
    <>
      {v.tdee(f.number(tdee.kcal), f.number(tdee.half_width))}
      <InfoTip label={i.tdee.label} title={i.tdee.title}>
        {i.tdee.text}
      </InfoTip>
      <Badge tone={trust === 'trusted' ? 'primary' : 'neutral'}>
        {trust === 'trusted' ? v.trusted : v.before}
      </Badge>
      <InfoTip label={i.trusted.label} title={i.trusted.title}>
        {i.trusted.text}
      </InfoTip>
    </>
  )
  // What there is beside the verdict: the TDEE, the trend weight, and
  // what was eaten against the target.
  const facts: { key: string; fact: ReactNode }[] = []
  if (measured !== false && missing === undefined) {
    facts.push({ key: 'tdee', fact: measured })
  }
  if (trend !== undefined) {
    facts.push({
      key: 'trend',
      fact: (
        <>
          {v.trend(f.number(trend.kg, 1))}
          <InfoTip label={i.trend.label} title={i.trend.title}>
            {i.trend.text}
          </InfoTip>
        </>
      ),
    })
  }
  if (verdict.intake_kcal !== undefined && verdict.target_kcal !== undefined) {
    facts.push({
      key: 'eaten',
      fact: v.eaten(f.number(verdict.intake_kcal), f.number(verdict.target_kcal)),
    })
  }

  return (
    <section
      aria-label={v.label}
      className={cn('space-y-4 rounded-lg border p-4 lg:p-6', look.card, entry)}
    >
      <div className="flex items-start gap-3">
        <Icon aria-hidden className={cn('mt-1 size-6 shrink-0 lg:size-7', look.tint)} />
        <div className="min-w-0 flex-1 space-y-1">
          <p className="text-small text-muted-foreground">{ofPhase ?? v.label}</p>
          {missing === undefined || measured === false ? (
            <p className="flex items-center gap-2 text-h2 lg:text-title">
              {headlines[state]}
              <InfoTip label={i.verdict.label} title={i.verdict.title}>
                {i.verdict.text}
              </InfoTip>
            </p>
          ) : (
            <p className="flex flex-wrap items-center gap-2 text-h2">{measured}</p>
          )}
          {said !== undefined && <p className="max-w-prose">{said}</p>}
          {recovered !== undefined && <p className="max-w-prose">{recovered}</p>}
        </div>
      </div>
      {facts.length > 0 && (
        <ul className="flex flex-wrap gap-x-5 gap-y-2 pl-9 text-small lg:pl-10">
          {facts.map(({ key, fact }) => (
            <li key={key} className="flex items-center gap-2">
              {fact}
            </li>
          ))}
        </ul>
      )}
      {missing !== undefined && <PlanRow missing={missing} />}
    </section>
  )
}

// rateSaid is the rate measured, in words, against the phase's range: of
// a loss or a gain, the range as the same (0.65–0.85); of a weight that
// goes the other way, signed (−0.85 to −0.65); of a phase that keeps it,
// around none (± 0.1).
function rateSaid(
  verdict: Verdict,
  within: boolean,
  v: Said,
  f: ReturnType<typeof useFormat>,
): string {
  const measured = verdict.measured_pct_week ?? 0
  const target = verdict.target_pct_week ?? 0
  const tolerance = verdict.tolerance_pct_week ?? 0
  const lo = target - tolerance
  const hi = target + tolerance
  let moving = v.steady
  if (measured < 0) {
    moving = v.losing(f.number(-measured, 2))
  } else if (measured > 0) {
    moving = v.gaining(f.number(measured, 2))
  }
  let range: string
  if (target === 0) {
    range = v.around(f.number(tolerance, 2))
  } else if ((lo >= 0 || hi <= 0) && measured * target >= 0) {
    range = v.range(
      f.number(Math.min(Math.abs(lo), Math.abs(hi)), 2),
      f.number(Math.max(Math.abs(lo), Math.abs(hi)), 2),
    )
  } else {
    range = v.span(f.signed(lo, 2), f.signed(hi, 2))
  }
  return within ? v.within(moving, range) : v.outside(moving, range)
}
