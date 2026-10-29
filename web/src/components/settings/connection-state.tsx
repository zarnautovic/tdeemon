import { useEffect, useState } from 'react'

import { $api } from '@/api/client'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// How often the access keys are asked again while one waits for its first
// call.
const pollEvery = 2000

// useNow is the time now, a second on each second while ticking.
function useNow(ticking: boolean): number {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    if (!ticking) {
      return undefined
    }
    const timer = setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      clearInterval(timer)
    }
  }, [ticking])
  return now
}

// ConnectionState says whether the access key with id was used yet, as
// the server says (its last use): waiting for the first call, asking the
// server again every two seconds, then connected, with how long ago the
// last call was, which counts on by itself. A screen reader hears when it
// connects, not each second.
export function ConnectionState({ id }: { id: string }) {
  const t = useMessages()
  const f = useFormat()
  const keys = $api.useQuery('get', '/api/v1/tokens', undefined, {
    refetchInterval: (query) =>
      query.state.data?.tokens.find((k) => k.id === id)?.last_used_at === undefined
        ? pollEvery
        : false,
  })
  const last = keys.data?.tokens.find((k) => k.id === id)?.last_used_at
  const now = useNow(last !== undefined)

  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-md border p-3 text-small">
      <span
        aria-hidden
        className={cn(
          'size-2 shrink-0 rounded-full',
          last === undefined ? 'bg-muted-foreground motion-safe:animate-pulse' : 'bg-success',
        )}
      />
      <output className="font-semibold">
        {last === undefined ? t.connect.waiting : t.connect.connected}
      </output>
      {last !== undefined && (
        <span className="text-muted-foreground">· {t.connect.lastCall(f.ago(last, now))}</span>
      )}
    </div>
  )
}
