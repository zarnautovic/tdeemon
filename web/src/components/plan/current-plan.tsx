import { useId } from 'react'

import type { components } from '@/api/schema'
import { useDate, usePlanWords } from '@/components/plan/proposal-text'
import { useMessages } from '@/i18n/messages'

type Plan = components['schemas']['PlanBody']

// CurrentPlan is the plan as it holds today: the phase and the target in
// force, by their numbers, the training days, and the phases and targets
// to come. Days are compared as YYYY-MM-DD, the user's today the server's.
export function CurrentPlan({ plan }: { plan: Plan }) {
  const t = useMessages()
  const id = useId()
  const date = useDate()
  const words = usePlanWords()
  const c = t.plan.current
  const today = plan.today

  // The phase of today: the last to start by today, unless it ended.
  const last = plan.phases.findLast((p) => p.start <= today)
  const phase =
    last !== undefined && (last.end === undefined || last.end >= today) ? last : undefined
  const target = plan.targets.findLast((tg) => tg.effective_from <= today)
  const ahead = [
    ...plan.phases
      .filter((p) => p.start > today)
      .map((p) => ({ day: p.start, text: words.phase(p) })),
    ...plan.targets
      .filter((tg) => tg.effective_from > today)
      .map((tg) => ({ day: tg.effective_from, text: words.target(tg) })),
  ].toSorted((a, b) => a.day.localeCompare(b.day))

  return (
    <section aria-labelledby={id} className="space-y-3 rounded-xl border p-4">
      <h2 id={id} className="text-lg font-semibold">
        {c.title}
      </h2>
      {plan.phases.length === 0 &&
      plan.targets.length === 0 &&
      plan.training_weekdays.length === 0 ? (
        <p className="text-sm text-muted-foreground">{c.none}</p>
      ) : (
        <>
          <dl className="space-y-2 text-sm">
            <div>
              <dt className="text-muted-foreground">{c.phase}</dt>
              <dd>
                {phase === undefined
                  ? c.nothing
                  : `${words.phase(phase)}, ${c.since(date(phase.start))}`}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{c.target}</dt>
              <dd>
                {target === undefined
                  ? c.nothing
                  : `${words.target(target)}, ${c.since(date(target.effective_from))}`}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t.plan.change.weekdays}</dt>
              <dd>{words.days(plan.training_weekdays)}</dd>
            </div>
          </dl>
          {ahead.length > 0 && (
            <div className="space-y-1 text-sm">
              <h3 className="text-muted-foreground">{c.ahead}</h3>
              <ul className="space-y-1">
                {ahead.map((a) => (
                  <li key={`${a.day} ${a.text}`}>{`${c.from(date(a.day))}: ${a.text}`}</li>
                ))}
              </ul>
            </div>
          )}
        </>
      )}
    </section>
  )
}
