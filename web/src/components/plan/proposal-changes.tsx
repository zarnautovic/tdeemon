import type { components } from '@/api/schema'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

import { useDate, useLimitWords, usePlanWords, useViolationText } from './proposal-text'

export type Proposal = components['schemas']['ProposalBody']
export type Preview = components['schemas']['PreviewBody']
type Changes = components['schemas']['ChangesBody']
type LimitWarning = components['schemas']['LimitWarningBody']

// ProposalChanges lists what a proposal changes of the plan, each change
// as it leaves it and, beside it, as it was; a limit, whether it loosens
// or tightens it.
export function ProposalChanges({ changes }: { changes: Changes }) {
  const t = useMessages()
  const date = useDate()
  const { phase, target, days } = usePlanWords()
  const limit = useLimitWords()
  const c = t.plan.change

  return (
    <ul className="space-y-2 text-sm">
      {changes.phases.map((ch) => (
        <li key={`phase ${ch.start}`}>
          <span className="block font-medium">
            {ch.before === undefined
              ? c.newPhase(date(ch.start))
              : ch.after === undefined
                ? c.deletedPhase(date(ch.start))
                : c.phase(date(ch.start))}
          </span>
          {ch.after !== undefined && <span className="block">{phase(ch.after)}</span>}
          {ch.before !== undefined && (
            <span className="block text-muted-foreground">{c.was(phase(ch.before))}</span>
          )}
        </li>
      ))}
      {changes.targets.map((ch) => (
        <li key={`target ${ch.from}`}>
          <span className="block font-medium">
            {ch.before === undefined
              ? c.newTarget(date(ch.from))
              : ch.after === undefined
                ? c.deletedTarget(date(ch.from))
                : c.target(date(ch.from))}
          </span>
          {ch.after !== undefined && <span className="block">{target(ch.after)}</span>}
          {ch.before !== undefined && (
            <span className="block text-muted-foreground">{c.was(target(ch.before))}</span>
          )}
        </li>
      ))}
      {changes.training_weekdays !== undefined && (
        <li>
          <span className="block font-medium">{c.weekdays}</span>
          <span className="block">{days(changes.training_weekdays.after)}</span>
          <span className="block text-muted-foreground">
            {c.was(days(changes.training_weekdays.before))}
          </span>
        </li>
      )}
      {changes.limits.map((ch) => (
        <li key={`limit ${ch.name}`}>
          <span className="block font-medium">{c.limit(limit.label(ch.name))}</span>
          <span className="block">
            {`${limit.value(ch.name, ch.after)}, ${ch.loosens ? c.loosened : c.tightened}`}
          </span>
          <span className="block text-muted-foreground">
            {c.was(limit.value(ch.name, ch.before))}
          </span>
        </li>
      ))}
    </ul>
  )
}

// PreviewLines says what a preview found: each target's average day with
// the rate it implies, its warnings, and what the limits refuse.
export function PreviewLines({ preview }: { preview: Preview }) {
  const t = useMessages()
  const f = useFormat()
  const violation = useViolationText()
  const date = useDate()
  const warning = (w: LimitWarning) =>
    t.plan.warnings[w.rule](f.number(w.value), f.number(w.threshold))

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
            <span
              key={`${w.rule} ${w.day_type ?? ''}`}
              className="block text-amber-700 dark:text-amber-400"
            >
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
