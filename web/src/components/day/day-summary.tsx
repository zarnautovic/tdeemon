import { Link } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import type { Day } from '@/api/types'
import { EnergyRing } from '@/components/day/energy-ring'
import { MacroBar, type Macro } from '@/components/day/macro-bar'
import { buttonVariants } from '@/components/ui/button'
import { daysBetween, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

type Verdict = components['schemas']['VerdictBody']

const macros: readonly Macro[] = ['protein', 'carbs', 'fat']

// The dot of a phase's line: the brand's green on target, the warning's
// amber off it; the words beside it say which.
const dots: Record<Verdict['rate'], string> = {
  none: 'bg-muted-foreground',
  on_target: 'bg-success',
  below: 'bg-warning',
  above: 'bg-warning',
}

// DaySummary is the day at a glance: what is left of its target in a ring,
// with what was eaten, the target, the TDEE calibration measures and the
// deficit (or surplus) the target makes of it; each macro against its
// target; and the phase the day is in, which leads to the progress. A day
// before any target shows what was eaten, and the way to a plan.
export function DaySummary({ day }: { day: Day }) {
  const t = useMessages()
  const f = useFormat()
  const { total, target } = day
  const summary = $api.useQuery('get', '/api/v1/summary')

  if (target === undefined) {
    return (
      <section aria-labelledby="day-summary" className="space-y-3 rounded-lg border bg-card p-4">
        <h2 id="day-summary" className="text-small text-muted-foreground">
          {t.day.eaten}
        </h2>
        <p className="text-title">{t.unit.kcal(f.number(total.kcal))}</p>
        <p className="text-small text-muted-foreground">
          {macros.map((m) => `${t.day.short[m]} ${f.grams(total[m])}`).join(' · ')}
        </p>
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md bg-surface-2 p-3">
          <p className="text-small">{t.day.summary.noTarget}</p>
          <Link to="/plan" className={buttonVariants({ variant: 'outline', size: 'sm' })}>
            {t.day.summary.setPlan}
          </Link>
        </div>
      </section>
    )
  }

  const tdee = summary.data?.tdee
  const measured = tdee?.measured === true ? tdee.kcal : undefined
  const balance = measured === undefined ? undefined : target.kcal - measured

  return (
    <section aria-labelledby="day-summary" className="space-y-4 rounded-lg border bg-card p-4">
      <h2 id="day-summary" className="sr-only">
        {t.day.summary.title}
      </h2>
      <div className="flex items-center gap-4">
        <EnergyRing eaten={total.kcal} target={target.kcal} />
        <dl className="grid max-w-72 flex-1 grid-cols-[1fr_auto] gap-x-3 gap-y-2 text-small">
          <dt className="text-muted-foreground">{t.day.eaten}</dt>
          <dd className="text-right font-semibold">{f.number(total.kcal)}</dd>
          <dt className="text-muted-foreground">{t.day.summary.target}</dt>
          <dd className="text-right font-semibold">{f.number(target.kcal)}</dd>
          {measured !== undefined && balance !== undefined && (
            <>
              <dt className="text-muted-foreground">{t.day.summary.tdee}</dt>
              <dd className="text-right font-semibold">
                {/* A TDEE calibration does not trust yet is about so much. */}
                {tdee?.trusted === true ? '' : '≈'}
                {f.number(measured)}
              </dd>
              <dt className="text-muted-foreground">
                {balance > 0 ? t.day.summary.surplus : t.day.summary.deficit}
              </dt>
              <dd className="text-right font-semibold">{f.signed(balance)}</dd>
            </>
          )}
        </dl>
      </div>
      <div className="space-y-3">
        {macros.map((m) => (
          <MacroBar key={m} macro={m} eaten={total[m]} target={target[m]} />
        ))}
      </div>
      <PhaseLine date={day.date} verdict={summary.data?.verdict} />
    </section>
  )
}

// PhaseLine is the phase of the plan a day is in, and its week of the
// phase's; of the phase now, the rate calibration measures and where it
// stands against the phase's. Nothing without a phase.
function PhaseLine({ date, verdict }: { date: string; verdict: Verdict | undefined }) {
  const t = useMessages()
  const f = useFormat()
  const plan = $api.useQuery('get', '/api/v1/plan')
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

  return (
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
}
