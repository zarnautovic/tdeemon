import { useQuery } from '@tanstack/react-query'

import { dayQuery } from '@/api/day'
import type { Metric } from '@/api/types'
import { addDays } from '@/i18n/format'
import limits from '@/lib/limits.json'

// usePreviousWeight is the weigh-in of the day before date, once the
// server said that day: what a weigh-in of date is near, for its field to
// hint, and whether a device sent it (an import), as a smart scale does
// every morning. It is undefined while the day comes, or when it did not
// come; weight is undefined when the day has none. Asked only when
// enabled, and of no day before the first a history is of.
export function usePreviousWeight(
  date: string,
  enabled = true,
): { weight: Metric | undefined } | undefined {
  const before = addDays(date, -1)
  const day = useQuery({ ...dayQuery(before), enabled: enabled && before >= limits.history.since })
  return day.isSuccess ? { weight: day.data.metrics.find((m) => m.kind === 'weight') } : undefined
}
