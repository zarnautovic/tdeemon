import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import type { Day } from '@/api/types'
import { usePreviousWeight } from '@/components/day/previous-weight'
import { WeighIn } from '@/components/day/weigh-in'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { clockAt } from '@/lib/zoned'

// noon ends the morning, on the user's clock.
const noon = 12

// morning tells whether an instant is in the morning on the user's clock:
// from the hour their day starts at to noon.
function morning(instant: number, timeZone: string, startHour: number): boolean {
  const hour = Number(clockAt(instant, timeZone).slice(0, 2))
  return hour >= startHour && hour < noon
}

// MorningWeighIn asks for the morning's weigh-in in a line of today's
// hero, a field and ✓: in the morning, while today has none, hinting with
// the day before's. A weigh-in, typed here or sent by a device, takes it
// away. A user whose weight a device sends, the day before's an import,
// is not asked; nor is one before the app knows the day before.
export function MorningWeighIn({ day }: { day: Day }) {
  const t = useMessages()
  const f = useFormat()
  const me = $api.useQuery('get', '/api/v1/me')
  const user = useQuery(sessionQuery).data?.user
  // When the page opened.
  const [now] = useState(Date.now)
  const asks =
    me.data?.today === day.date &&
    !day.metrics.some((m) => m.kind === 'weight') &&
    user !== undefined &&
    morning(now, user.timezone, user.day_start_hour)
  const before = usePreviousWeight(day.date, asks)
  if (!asks || before === undefined || before.weight?.source === 'import') {
    return null
  }

  return (
    <WeighIn
      date={day.date}
      label={t.day.weighIn.morning}
      placeholder={before.weight === undefined ? undefined : f.field(before.weight.value, 1)}
      className="rounded-md bg-surface-2 px-3 py-2"
    />
  )
}
