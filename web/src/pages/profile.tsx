import { useId, useState } from 'react'

import { $api } from '@/api/client'
import { LoadError } from '@/components/load-error'
import { ProfileForm } from '@/components/plan/profile-form'
import { Skeleton } from '@/components/skeleton'
import { SubPage } from '@/components/sub-page'
import { useMessages } from '@/i18n/messages'

// ProfilePage sets what a plan needs of the user: their sex, birth date
// and height. Only the user sets them, in a web session, as a plan rests
// on them (docs/limits.md).
export function ProfilePage() {
  const t = useMessages()
  const id = useId()
  const profile = $api.useQuery('get', '/api/v1/profile')
  // The profile as the server gave it since the page opened, not before.
  const [opened] = useState(Date.now)
  const fresh = profile.data !== undefined && profile.dataUpdatedAt >= opened

  return (
    <SubPage title={t.plan.profile} titleId={id} from="plan" hint={t.plan.profileHint}>
      {/* The profile as the server has it when the page opens, not as a
          cache kept it. Once shown, what is typed stays, saved or not,
          whatever a later ask comes to. Loading too while a cached
          profile is asked again. */}
      {fresh ? (
        <ProfileForm profile={profile.data} labelledBy={id} />
      ) : profile.isError ? (
        <LoadError
          title={t.loadFailed.profile}
          error={profile.error}
          onRetry={() => void profile.refetch()}
        />
      ) : (
        <Skeleton blocks={['h-24', 'h-16', 'h-16', 'h-11 w-28']} />
      )}
    </SubPage>
  )
}
