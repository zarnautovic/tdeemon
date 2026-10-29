import { useId } from 'react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { kcalShift } from '@/components/plan/change-words'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

type Plan = components['schemas']['PlanBody']
type Proposal = components['schemas']['ProposalBody']

// ProposalData is what the data say beside a proposal, from calibration's
// summary: the pace it measures against the phase's range, and how far
// its TDEE is to be trusted. When the proposal moves the kcal where the
// data do not ask for it (the pace is in range, or they ask the other
// way), it says so, so the user weighs the coach's words against what was
// measured.
export function ProposalData({ proposal, plan }: { proposal: Proposal; plan: Plan }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const summary = $api.useQuery('get', '/api/v1/summary')
  if (summary.data === undefined) {
    return null
  }
  const { verdict, tdee } = summary.data
  const d = t.plan.pending.data
  const pct = (n: number) => f.percent(n, 2)
  const measured = verdict.measured_pct_week
  const target = verdict.target_pct_week
  const tolerance = verdict.tolerance_pct_week
  const shift = kcalShift(proposal, plan.targets, plan.training_weekdays.length)
  let against: string | undefined
  if (shift !== 0 && verdict.rate === 'on_target') {
    against = d.inRange
  } else if (shift < 0 && verdict.gap_kcal > 0) {
    against = d.more
  } else if (shift > 0 && verdict.gap_kcal < 0) {
    against = d.fewer
  }

  return (
    <section aria-labelledby={id} className="space-y-1 rounded-md border p-3 text-sm">
      <h3 id={id} className="font-medium">
        {d.title}
      </h3>
      <p>
        {verdict.rate === 'none' ||
        measured === undefined ||
        target === undefined ||
        tolerance === undefined
          ? d.noVerdict
          : d.rate[verdict.rate](pct(measured), pct(target - tolerance), pct(target + tolerance))}
      </p>
      <p className="text-muted-foreground">
        {!tdee.measured || tdee.kcal === undefined
          ? d.noTdee
          : tdee.trusted
            ? d.trusted(f.number(tdee.kcal), f.number(tdee.half_width ?? 0))
            : d.notTrusted(f.number(tdee.kcal))}
      </p>
      {against !== undefined && <p className="font-medium text-warning">{against}</p>}
    </section>
  )
}
