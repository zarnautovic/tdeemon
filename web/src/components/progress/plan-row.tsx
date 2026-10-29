import { Link } from '@tanstack/react-router'

import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// PlanRow is what a verdict on the phase waits for, under what the hero
// says of the TDEE: a phase of the plan (no_phase) or its target
// (no_target), and the way to the plan.
export function PlanRow({ missing }: { missing: 'no_phase' | 'no_target' }) {
  const v = useMessages().charts.verdict

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-md bg-surface-2 p-3">
      <p className="max-w-prose text-small">
        <b className="font-semibold">{missing === 'no_phase' ? v.noPhase : v.noTarget}</b>{' '}
        {v.reasons[missing]}
      </p>
      <Link to="/plan" className={buttonVariants({ variant: 'outline', size: 'sm' })}>
        {missing === 'no_phase' ? v.setPlan : v.openPlan}
      </Link>
    </div>
  )
}
