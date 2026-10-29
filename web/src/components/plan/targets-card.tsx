import { useId } from 'react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { EditTargets } from '@/components/plan/edit-targets'
import { targetNow, weekdays, weeklyAverage } from '@/components/plan/phases'
import { useDate, usePlanWords } from '@/components/plan/proposal-text'
import { InfoTip } from '@/components/ui/info-tip'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

type Plan = components['schemas']['PlanBody']
type DayTarget = components['schemas']['DayTargetBody']

// TargetsCard is what the plan sets a day to eat by its type, training
// and rest: the kcal and the macros of each, and the days of the week
// each is; the weekly average they come to over the training days,
// against the TDEE calibration measures; and the targets to come. The
// user edits them from today on.
export function TargetsCard({ plan }: { plan: Plan }) {
  const t = useMessages()
  const date = useDate()
  const words = usePlanWords()
  const id = useId()
  const summary = $api.useQuery('get', '/api/v1/summary')
  const g = t.plan.targets
  const target = targetNow(plan.targets, plan.today)
  const ahead = plan.targets.filter((tg) => tg.effective_from > plan.today)
  const training = weekdays.filter((d) => plan.training_weekdays.includes(d))
  const rest = weekdays.filter((d) => !plan.training_weekdays.includes(d))

  return (
    <section aria-labelledby={id} className="space-y-3 rounded-lg border bg-card p-4 lg:p-5">
      <header className="flex items-center justify-between gap-2">
        <h2 id={id} className="text-h3">
          {g.title}
        </h2>
        <div className="flex items-center gap-1">
          {target !== undefined && (
            <span className="text-small text-muted-foreground">
              {g.since(date(target.effective_from))}
            </span>
          )}
          <EditTargets plan={plan} />
        </div>
      </header>
      {target === undefined ? (
        <>
          <p className="text-small text-muted-foreground">{g.none}</p>
          <p className="text-small">
            <span className="text-muted-foreground">{g.trainingDays}: </span>
            {words.days(training)}
          </p>
        </>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3">
            <DayTile name={g.training} day={target.training} days={training} />
            <DayTile name={g.rest} day={target.rest} days={rest} />
          </div>
          <Average
            kcal={weeklyAverage(target.training.kcal, target.rest.kcal, training.length)}
            tdee={summary.data?.tdee}
          />
        </>
      )}
      {ahead.length > 0 && (
        <ul className="space-y-1 border-t pt-3 text-small">
          {ahead.map((tg) => (
            <li key={tg.effective_from}>
              <span className="text-muted-foreground">{g.from(date(tg.effective_from))}: </span>
              {words.target(tg)}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

// DayTile is a type of day's target: its kcal, its macros, and the days
// of the week it is.
function DayTile({
  name,
  day,
  days,
}: {
  name: string
  day: DayTarget
  days: readonly (typeof weekdays)[number][]
}) {
  const t = useMessages()
  const f = useFormat()
  const words = usePlanWords()
  const g = t.plan.targets
  const s = t.day.short

  return (
    <div className="min-w-0 space-y-1 rounded-md bg-surface-2 p-3">
      <h3 className="text-small text-muted-foreground">{name}</h3>
      <p className="text-h2">{t.unit.kcal(f.number(day.kcal))}</p>
      <p className="text-caption">
        {s.protein} {f.grams(day.protein)} · {s.carbs} {f.grams(day.carbs)} · {s.fat}{' '}
        {f.grams(day.fat)} g
      </p>
      <p className="text-caption text-muted-foreground">
        {days.length === 7
          ? g.everyDay
          : days.length === 0
            ? g.noDay
            : `${g.days(days.length)} · ${words.days([...days])}`}
      </p>
    </div>
  )
}

// Average is the weekly average of the targets, a day, against the TDEE
// calibration measures, about so much while calibration does not trust it
// yet; and the deficit or surplus it plans, said in words beside a number
// never below 0. The TDEE and the deficit are each an ⓘ away from what
// they are.
function Average({
  kcal,
  tdee,
}: {
  kcal: number
  tdee: components['schemas']['TdeeBody'] | undefined
}) {
  const t = useMessages()
  const f = useFormat()
  const g = t.plan.targets
  const measured = tdee?.measured === true ? tdee.kcal : undefined
  const balance = measured === undefined ? undefined : Math.round(kcal - measured)

  return (
    <dl className="grid grid-cols-[1fr_auto] items-center gap-x-3 gap-y-1 text-small">
      <dt className="text-muted-foreground">{g.average}</dt>
      <dd className="text-right font-semibold">{g.perDay(f.number(kcal))}</dd>
      <dt className="flex items-center gap-1 text-muted-foreground">
        {g.tdee}
        <InfoTip label={t.info.tdee.label} title={t.info.tdee.title}>
          {t.info.tdee.text}
        </InfoTip>
      </dt>
      {measured === undefined ? (
        <dd className="text-right text-muted-foreground">{g.noTdee}</dd>
      ) : (
        <dd className="text-right font-semibold">
          {/* A TDEE calibration does not trust yet is about so much. */}
          {tdee?.trusted === true ? '' : '≈'}
          {g.perDay(f.number(measured))}
        </dd>
      )}
      {balance !== undefined && (
        <>
          <dt className="flex items-center gap-1 text-muted-foreground">
            {balance > 0 ? g.surplus : balance < 0 ? g.deficit : g.even}
            <InfoTip label={t.info.deficit.label} title={t.info.deficit.title}>
              {t.info.deficit.text}
            </InfoTip>
          </dt>
          <dd className="text-right font-semibold">{g.balance(f.number(Math.abs(balance)))}</dd>
        </>
      )}
    </dl>
  )
}
