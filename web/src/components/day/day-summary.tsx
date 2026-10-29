import { Link } from '@tanstack/react-router'

import { $api } from '@/api/client'
import type { Day } from '@/api/types'
import { CalibrationRow } from '@/components/day/calibration-row'
import { DayPhase } from '@/components/day/day-phase'
import { EnergyRing } from '@/components/day/energy-ring'
import { kcalLeft, kcalStanding } from '@/components/day/left'
import { MacroBar, type Macro } from '@/components/day/macro-bar'
import { MorningWeighIn } from '@/components/day/morning-weigh-in'
import { weekOf } from '@/components/day/week'
import { buttonVariants } from '@/components/ui/button'
import { InfoTip } from '@/components/ui/info-tip'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

const macros: readonly Macro[] = ['protein', 'carbs', 'fat']

// A term of the numbers beside the ring and its value: in two columns on
// a phone, the term and its value on a row; on a desktop, each a column
// of its own, the value under the term.
const term = 'contents lg:flex lg:flex-col lg:gap-1'
const value = 'text-right font-semibold lg:text-left lg:text-h2'

// DaySummary is the day at a glance: what is left of its target in a ring,
// with what was eaten, the target, the TDEE calibration measures and what
// is left of it after what was eaten, live through the day (or, of a day
// before, how far under it the day stayed), each an ⓘ away from what it
// is, and until calibration has a TDEE, how far it is from a first; each
// macro against its target; and the phase the day is in, which leads to
// the progress, unless the page shows it elsewhere (phase: a desktop's,
// beside the day). A desktop has the numbers side by side beside the
// ring, and the macros in three columns. A day before that went over its
// target says it in a sentence beside the TDEE, not in an error's colour.
// A day before any target shows what was eaten, and the way to a plan.
export function DaySummary({ day, phase = true }: { day: Day; phase?: boolean }) {
  const t = useMessages()
  const f = useFormat()
  const { total, target } = day
  const summary = $api.useQuery('get', '/api/v1/summary')
  const me = $api.useQuery('get', '/api/v1/me')
  const today = me.data?.today
  const past = today !== undefined && day.date < today
  // A day before's TDEE is the one it had then, from the history of its
  // week, which the week strip has asked for; today's, the summary's.
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query: weekOf(day.date, today ?? day.date) } },
    { enabled: past },
  )
  // Calibration on its way to a first TDEE, in the TDEE's place: now's,
  // so not of a day before.
  const calibrating = !past && summary.data?.tdee.measured === false ? summary.data.tdee : undefined

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
        {calibrating !== undefined && (
          <dl className="space-y-1 text-small">
            <CalibrationRow tdee={calibrating} />
          </dl>
        )}
        <NoTarget date={day.date} />
      </section>
    )
  }

  const s = t.day.summary
  const tdee = past
    ? history.data?.days.find((d) => d.date === day.date)?.tdee
    : summary.data?.tdee.measured === true
      ? summary.data.tdee
      : undefined
  const measured = tdee?.kcal
  // What the TDEE leaves after what was eaten, both as shown; below 0
  // past it.
  const left = measured === undefined ? undefined : kcalLeft(total.kcal, measured)
  const over = -kcalLeft(total.kcal, target.kcal)
  let leftTerm = s.leftToTdee
  if (left !== undefined && left < 0) {
    leftTerm = s.overTdee
  } else if (past) {
    leftTerm = s.underTdee
  }

  return (
    <section
      aria-labelledby="day-summary"
      className="space-y-4 rounded-lg border bg-card p-4 lg:p-6"
    >
      <h2 id="day-summary" className="sr-only">
        {s.title}
      </h2>
      <MorningWeighIn day={day} />
      <div className="flex items-center gap-4 lg:gap-8">
        <EnergyRing eaten={total.kcal} target={target.kcal} past={past} />
        <dl className="grid max-w-72 flex-1 grid-cols-[1fr_auto] items-center gap-x-3 gap-y-2 text-small lg:max-w-none lg:grid-cols-4 lg:items-start lg:gap-x-6">
          <div className={term}>
            <dt className="text-muted-foreground">{t.day.eaten}</dt>
            <dd className={value}>{f.number(total.kcal)}</dd>
          </div>
          <div className={term}>
            <dt className="text-muted-foreground">{s.target}</dt>
            <dd className={value}>{f.number(target.kcal)}</dd>
          </div>
          {measured !== undefined && left !== undefined && (
            <>
              <div className={term}>
                <dt className="flex items-center gap-1 text-muted-foreground">
                  {s.tdee}
                  <InfoTip label={t.info.tdee.label} title={t.info.tdee.title}>
                    {t.info.tdee.text}
                  </InfoTip>
                </dt>
                <dd className={value}>
                  {/* A TDEE calibration does not trust yet is about so much. */}
                  {tdee?.trusted === true ? '' : '≈'}
                  {f.number(measured)}
                </dd>
              </div>
              <div className={term}>
                <dt className="flex items-center gap-1 text-muted-foreground">
                  {leftTerm}
                  <InfoTip label={t.info.left.label} title={t.info.left.title}>
                    {t.info.left.text}
                  </InfoTip>
                </dt>
                <dd className={value}>{f.number(Math.abs(left))}</dd>
              </div>
            </>
          )}
          {calibrating !== undefined && (
            <div className="col-span-2 space-y-1">
              <CalibrationRow tdee={calibrating} />
            </div>
          )}
        </dl>
      </div>
      {past && left !== undefined && kcalStanding(total.kcal, target.kcal) === 'over' && (
        <p className="text-small">
          {left < 0
            ? s.overOverTdee(f.number(over), f.number(-left))
            : s.overUnderTdee(f.number(over), f.number(left))}
        </p>
      )}
      <div className="space-y-3 lg:grid lg:grid-cols-3 lg:gap-6 lg:space-y-0">
        {macros.map((m) => (
          <MacroBar key={m} macro={m} eaten={total[m]} target={target[m]} />
        ))}
      </div>
      {phase && <DayPhase date={day.date} />}
    </section>
  )
}

// NoTarget says why a day has no target, with the way to the plan: a day
// before the plan's first target, when the plan starts; a plan of phases
// without a target yet, that it gives none; else, before a plan, that a
// plan gives the day one.
function NoTarget({ date }: { date: string }) {
  const t = useMessages()
  const f = useFormat()
  const s = t.day.summary
  const plan = $api.useQuery('get', '/api/v1/plan')
  const first = plan.data?.targets[0]?.effective_from
  let words = s.noTarget
  let link = s.setPlan
  if (first !== undefined && date < first) {
    words = s.startsOn(f.short(first))
    link = s.seePlan
  } else if (first === undefined && (plan.data?.phases.length ?? 0) > 0) {
    words = s.noTargetYet
    link = s.seePlan
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-md bg-surface-2 p-3">
      <p className="text-small">{words}</p>
      <Link to="/plan" className={buttonVariants({ variant: 'outline', size: 'sm' })}>
        {link}
      </Link>
    </div>
  )
}
