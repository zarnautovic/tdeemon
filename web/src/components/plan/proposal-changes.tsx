import type { components } from '@/api/schema'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

import { priorOf, useChangeWords, useWarningText } from './change-words'
import { useDate, useLimitWords, usePlanWords, useViolationText } from './proposal-text'

export type Proposal = components['schemas']['ProposalBody']
export type Preview = components['schemas']['PreviewBody']
type Changes = components['schemas']['ChangesBody']
type Target = components['schemas']['TargetPeriodBody']

// ProposalChanges lists what a proposal changes of the plan, and only
// that: each value from what it was to what it is to be ("Rest days:
// 1,950 → 1,850 kcal"); a new phase, or a first target, whole, and one
// deleted as it was; a limit, whether it loosens or tightens it. A new
// target is told against the one in force before its day, by the plan's
// targets.
export function ProposalChanges({
  changes,
  targets,
}: {
  changes: Changes
  targets?: readonly Target[] | undefined
}) {
  const t = useMessages()
  const date = useDate()
  const { phase, target, days } = usePlanWords()
  const words = useChangeWords()
  const limit = useLimitWords()
  const c = t.plan.change

  return (
    <ul className="space-y-2 text-sm">
      {changes.phases.map((ch) => (
        <li key={`phase ${ch.start}`}>
          {ch.before === undefined || ch.after === undefined ? (
            <>
              <span className="block font-medium">
                {ch.before === undefined
                  ? c.newPhase(date(ch.start))
                  : c.deletedPhase(date(ch.start))}
              </span>
              {ch.after !== undefined && <span className="block">{phase(ch.after)}</span>}
              {ch.before !== undefined && (
                <span className="block text-muted-foreground">{c.was(phase(ch.before))}</span>
              )}
            </>
          ) : (
            <>
              <span className="block font-medium">{c.phase(date(ch.start))}</span>
              <span className="block">{words.phase(ch.before, ch.after).join(' · ')}</span>
            </>
          )}
        </li>
      ))}
      {changes.targets.map((ch) => {
        const prior = priorOf(ch, targets)
        return (
          <li key={`target ${ch.from}`}>
            {ch.after === undefined ? (
              <>
                <span className="block font-medium">{c.deletedTarget(date(ch.from))}</span>
                {ch.before !== undefined && (
                  <span className="block text-muted-foreground">{c.was(target(ch.before))}</span>
                )}
              </>
            ) : prior === undefined ? (
              <>
                <span className="block font-medium">{c.newTarget(date(ch.from))}</span>
                <span className="block">{target(ch.after)}</span>
              </>
            ) : (
              <>
                <span className="block font-medium">{c.target(date(ch.from))}</span>
                {words.target(prior, ch.after).map((line) => (
                  <span key={line.days} className="block">
                    {`${words.upper(c.days[line.days])}: ${line.parts.join(' · ')}`}
                  </span>
                ))}
              </>
            )}
          </li>
        )
      })}
      {changes.training_weekdays !== undefined && (
        <li>
          <span className="block font-medium">{c.weekdays}</span>
          <span className="block">
            {c.to(days(changes.training_weekdays.before), days(changes.training_weekdays.after))}
          </span>
        </li>
      )}
      {changes.limits.map((ch) => (
        <li key={`limit ${ch.name}`}>
          <span className="block font-medium">{c.limit(limit.label(ch.name))}</span>
          <span className="block">
            {`${c.to(limit.value(ch.name, ch.before), limit.value(ch.name, ch.after))}, ${ch.loosens ? c.loosened : c.tightened}`}
          </span>
        </li>
      ))}
    </ul>
  )
}

// PreviewLines says what a preview found: each target's average day with
// the rate it implies, its warnings in plain words, of the type of day
// they are of, and what the limits refuse.
export function PreviewLines({ preview }: { preview: Preview }) {
  const t = useMessages()
  const f = useFormat()
  const violation = useViolationText()
  const date = useDate()
  const warning = useWarningText()

  return (
    <ul className="space-y-1 text-sm">
      {preview.checks.map((ck) => (
        <li key={`${ck.of} ${ck.day}`}>
          {ck.weekly_kcal !== undefined && (
            <span className="block">
              {t.plan.pending.average(date(ck.day), f.number(ck.weekly_kcal))}
              {ck.expected_pct_week !== undefined &&
                ck.expected_kg_week !== undefined &&
                t.plan.pending.rate(
                  f.percent(ck.expected_pct_week, 2),
                  f.number(ck.expected_kg_week, 2),
                )}
            </span>
          )}
          {ck.warnings.map((w) => (
            <span key={`${w.rule} ${w.day_type ?? ''}`} className="block text-warning">
              {warning(w)}
            </span>
          ))}
          {ck.violations.map((v) => (
            <span key={`${v.rule} ${v.day_type ?? ''}`} className="block text-destructive">
              {violation(v)}
            </span>
          ))}
        </li>
      ))}
    </ul>
  )
}
