import { Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'
import { useState } from 'react'

import { $api } from '@/api/client'
import { LoadError } from '@/components/load-error'
import { CurrentPlan } from '@/components/plan/current-plan'
import { LimitsForm } from '@/components/plan/limits-form'
import { PendingProposal } from '@/components/plan/pending-proposal'
import { ProfileForm } from '@/components/plan/profile-form'
import { Revisions } from '@/components/plan/revisions'
import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// PlanPage is the user's plan in the app: the plan as it holds today, a
// proposed change that waits for their answer, the history of the plan's
// changes, what a plan needs
// of the user, their profile, and how far it may go, their limits. The
// profile and the limits change only in a web session, as a plan rests on
// them (docs/limits.md).
export function PlanPage() {
  const t = useMessages()
  const profile = $api.useQuery('get', '/api/v1/profile')
  // The profile as the server gave it since the page opened, not before.
  const [opened] = useState(Date.now)
  const fresh = profile.data !== undefined && profile.dataUpdatedAt >= opened
  const limits = $api.useQuery('get', '/api/v1/limits')
  const plan = $api.useQuery('get', '/api/v1/plan')

  return (
    <section className="space-y-6">
      <Link to="/settings" className={buttonVariants({ variant: 'ghost', className: '-ml-2' })}>
        <ChevronLeft aria-hidden />
        {t.plan.back}
      </Link>
      <h1 className="text-2xl font-semibold">{t.plan.title}</h1>
      {plan.isError && <LoadError error={plan.error} onRetry={() => void plan.refetch()} />}
      {plan.data !== undefined && <CurrentPlan plan={plan.data} />}
      {plan.data?.pending !== undefined && (
        // A proposal in place of another starts afresh, without the
        // other's answer.
        <PendingProposal
          key={plan.data.pending.id}
          proposal={plan.data.pending}
          preview={plan.data.pending_preview}
        />
      )}
      {plan.data !== undefined && <Revisions revisions={plan.data.revisions} />}
      {/* Loading too while a cached profile is asked again. */}
      {!fresh && !profile.isError && (
        <output className="block text-muted-foreground">{t.loading}</output>
      )}
      {/* The profile as the server has it when the page opens, not as a
          cache kept it. Once shown, what is typed stays, saved or not,
          whatever a later ask comes to. */}
      {fresh ? (
        <ProfileForm profile={profile.data} />
      ) : (
        profile.isError && (
          <LoadError error={profile.error} onRetry={() => void profile.refetch()} />
        )
      )}
      {limits.isPending && <output className="block text-muted-foreground">{t.loading}</output>}
      {limits.isError && <LoadError error={limits.error} onRetry={() => void limits.refetch()} />}
      {limits.data && <LimitsForm body={limits.data} />}
    </section>
  )
}
