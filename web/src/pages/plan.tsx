import { Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'
import { useState } from 'react'

import { $api } from '@/api/client'
import { LoadError } from '@/components/load-error'
import { LimitsForm } from '@/components/plan/limits-form'
import { ProfileForm } from '@/components/plan/profile-form'
import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// PlanPage is what a plan needs of the user, their profile, and how far
// it may go, their limits; both change only in a web session, as a plan
// rests on them (docs/limits.md).
export function PlanPage() {
  const t = useMessages()
  const profile = $api.useQuery('get', '/api/v1/profile')
  // The profile as the server gave it since the page opened, not before.
  const [opened] = useState(Date.now)
  const fresh = profile.data !== undefined && profile.dataUpdatedAt >= opened
  const limits = $api.useQuery('get', '/api/v1/limits')

  return (
    <section className="space-y-6">
      <Link to="/settings" className={buttonVariants({ variant: 'ghost', className: '-ml-2' })}>
        <ChevronLeft aria-hidden />
        {t.plan.back}
      </Link>
      <h1 className="text-2xl font-semibold">{t.plan.title}</h1>
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
