import { Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'

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
  const limits = $api.useQuery('get', '/api/v1/limits')

  return (
    <section className="space-y-6">
      <Link to="/settings" className={buttonVariants({ variant: 'ghost', className: '-ml-2' })}>
        <ChevronLeft aria-hidden />
        {t.plan.back}
      </Link>
      <h1 className="text-2xl font-semibold">{t.plan.title}</h1>
      {profile.isPending && <output className="block text-muted-foreground">{t.loading}</output>}
      {profile.isError && (
        <LoadError error={profile.error} onRetry={() => void profile.refetch()} />
      )}
      {/* The profile as the server has it when the page opens, not as a
          cache kept it. What is typed then stays, saved or not. */}
      {profile.isSuccess && profile.isFetchedAfterMount && <ProfileForm profile={profile.data} />}
      {limits.isPending && <output className="block text-muted-foreground">{t.loading}</output>}
      {limits.isError && <LoadError error={limits.error} onRetry={() => void limits.refetch()} />}
      {limits.data && <LimitsForm body={limits.data} />}
    </section>
  )
}
