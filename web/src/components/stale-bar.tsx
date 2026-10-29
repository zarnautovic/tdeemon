import { notifyManager, useQuery, useQueryClient, type QueryCache } from '@tanstack/react-query'
import { CloudOff } from 'lucide-react'
import { useState, useSyncExternalStore } from 'react'

import { NoAnswerError, statusOf } from '@/api/errors'
import { sessionQuery } from '@/api/session'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { dateAt } from '@/lib/zoned'

// A Stale is what the data a page shows came to when asking for them
// again failed: the oldest's time, and whether the server was out of
// reach (offline) or answered with a failure of its own.
interface Stale {
  since: number
  offline: boolean
}

// staleOf is the Stale of the queries a page shows (active) that keep
// their data though asking for them again failed for want of an answer
// or with the server failing, or waits for the network to come back.
// A refusal (4xx) is not the network's, and its part says it.
function staleOf(cache: QueryCache): Stale | undefined {
  let stale: Stale | undefined
  for (const query of cache.findAll({ type: 'active' })) {
    const { data, dataUpdatedAt, error, fetchStatus, status } = query.state
    const paused = fetchStatus === 'paused'
    const offline = paused || error instanceof NoAnswerError
    const failed = status === 'error' && (offline || (statusOf(error) ?? 0) >= 500)
    if (data === undefined || (!paused && !failed)) {
      continue
    }
    stale = {
      since: Math.min(stale?.since ?? dataUpdatedAt, dataUpdatedAt),
      offline: (stale?.offline ?? false) || offline,
    }
  }
  return stale
}

// useStale is the Stale of the page's data, while they are; each part a
// value of its own, as the store compares them by value.
function useStale(): Stale | undefined {
  const cache = useQueryClient().getQueryCache()
  const subscribe = (changed: () => void) => cache.subscribe(notifyManager.batchCalls(changed))
  const since = useSyncExternalStore(subscribe, () => staleOf(cache)?.since)
  const offline = useSyncExternalStore(subscribe, () => staleOf(cache)?.offline ?? false)
  return since === undefined ? undefined : { since, offline }
}

// StaleBar is a thin bar over a page that keeps showing its data when
// asking for them again failed, rather than an error in their place:
// offline (or the server failing), and the time of the oldest of them
// on the user's clock. It goes once they all came again.
export function StaleBar() {
  const stale = useStale()

  return (
    <output className="block">
      {/* Since another time, another bar. */}
      {stale !== undefined && <Bar key={stale.since} stale={stale} />}
    </output>
  )
}

// Bar is the bar's line: with the date too when the data are of a day
// before the one the bar came on.
function Bar({ stale }: { stale: Stale }) {
  const t = useMessages()
  const f = useFormat()
  const timeZone = useQuery(sessionQuery).data?.user.timezone ?? 'UTC'
  // When the bar came.
  const [now] = useState(Date.now)
  const at = new Date(stale.since).toISOString()
  const time = f.time(at, timeZone)
  const when =
    dateAt(stale.since, timeZone) === dateAt(now, timeZone)
      ? time
      : t.stale.dayTime(f.day(at, timeZone, now), time)

  return (
    <span className="mb-4 flex items-center gap-2 rounded-md bg-surface-2 px-3 py-1.5 text-small">
      <CloudOff aria-hidden className="size-4 shrink-0 text-muted-foreground" />
      {(stale.offline ? t.stale.offline : t.stale.failed)(when)}
    </span>
  )
}
