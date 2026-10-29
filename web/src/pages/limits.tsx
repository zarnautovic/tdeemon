import { useId } from 'react'

import { $api } from '@/api/client'
import { LoadError } from '@/components/load-error'
import { LimitsForm } from '@/components/plan/limits-form'
import { Skeleton } from '@/components/skeleton'
import { SubPage } from '@/components/sub-page'
import { useMessages } from '@/i18n/messages'

// LimitsPage changes how far a plan may go, the user's limits, in a web
// session only (docs/limits.md).
export function LimitsPage() {
  const t = useMessages()
  const id = useId()
  const limits = $api.useQuery('get', '/api/v1/limits')

  return (
    <SubPage title={t.plan.limits} titleId={id} from="plan" hint={t.plan.limitsHint}>
      {limits.isPending && <Skeleton blocks={['h-14', 'h-14', 'h-14', 'h-11 w-28']} />}
      {/* The limits that came stay while asking for them again fails. */}
      {limits.data === undefined && limits.isError && (
        <LoadError
          title={t.loadFailed.limits}
          error={limits.error}
          onRetry={() => void limits.refetch()}
        />
      )}
      {limits.data && <LimitsForm body={limits.data} labelledBy={id} />}
    </SubPage>
  )
}
