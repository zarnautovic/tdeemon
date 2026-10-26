import type { components } from '@/api/schema'
import { useFormat } from '@/i18n/format'
import { useLocale } from '@/i18n/locale'
import { useMessages } from '@/i18n/messages'

import { useViolationText } from './proposal-text'

export type Proposal = components['schemas']['ProposalBody']
export type Preview = components['schemas']['PreviewBody']
type Changes = components['schemas']['ChangesBody']
type Phase = components['schemas']['PhaseBody']
type TargetValues = components['schemas']['TargetValuesBody']
type DayTarget = components['schemas']['DayTargetBody']
type LimitWarning = components['schemas']['LimitWarningBody']

const weekdays = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']

// useWeekdayName names a day of the week, as the API names it (monday),
// short in the app's language.
function useWeekdayName(): (name: string) => string {
  const locale = useLocale()
  const format = new Intl.DateTimeFormat(locale, { weekday: 'short', timeZone: 'UTC' })
  // 2026-10-25 was a Sunday.
  return (name) => format.format(Date.UTC(2026, 9, 25 + weekdays.indexOf(name)))
}

// ProposalChanges lists what a proposal changes of the plan, each change
// as it leaves it and, beside it, as it was.
export function ProposalChanges({ changes }: { changes: Changes }) {
  const t = useMessages()
  const f = useFormat()
  const weekdayName = useWeekdayName()
  const c = t.plan.change

  const phase = (p: Phase) =>
    [
      `${c.types[p.type]}, ${c.rate(f.percent(p.rate_pct_week, 2))}`,
      ...(p.end === undefined ? [] : [c.until(f.short(p.end))]),
      ...(p.goal_weight === undefined ? [] : [c.goal(f.number(p.goal_weight, 1))]),
      ...(p.milestone_weight === undefined ? [] : [c.milestone(f.number(p.milestone_weight, 1))]),
    ].join(', ')
  const day = (d: DayTarget) =>
    c.day(f.number(d.kcal), f.number(d.protein, 1), f.number(d.carbs, 1), f.number(d.fat, 1))
  // target is a target's values: once for an unsplit target, else each
  // type of day's.
  const target = (v: TargetValues) =>
    day(v.training) === day(v.rest)
      ? day(v.training)
      : `${c.training}: ${day(v.training)}; ${c.rest}: ${day(v.rest)}`
  const days = (names: string[]) =>
    names.length === 0 ? c.noDays : names.map((n) => weekdayName(n)).join(', ')

  return (
    <ul className="space-y-2 text-sm">
      {changes.phases.map((ch) => (
        <li key={`phase ${ch.start}`}>
          <span className="block font-medium">
            {ch.before === undefined
              ? c.newPhase(f.short(ch.start))
              : ch.after === undefined
                ? c.deletedPhase(f.short(ch.start))
                : c.phase(f.short(ch.start))}
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
              ? c.newTarget(f.short(ch.from))
              : ch.after === undefined
                ? c.deletedTarget(f.short(ch.from))
                : c.target(f.short(ch.from))}
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
    </ul>
  )
}

// PreviewLines says what a preview found: each target's average day with
// the rate it implies, its warnings, and what the limits refuse.
export function PreviewLines({ preview }: { preview: Preview }) {
  const t = useMessages()
  const f = useFormat()
  const violation = useViolationText()
  const warning = (w: LimitWarning) =>
    t.plan.warnings[w.rule](f.number(w.value), f.number(w.threshold))

  return (
    <ul className="space-y-1 text-sm">
      {preview.checks.map((ck) => (
        <li key={`${ck.of} ${ck.day}`}>
          {ck.weekly_kcal !== undefined && (
            <span className="block">
              {t.plan.pending.average(f.short(ck.day), f.number(ck.weekly_kcal))}
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
