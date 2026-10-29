import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import type { Metric } from '@/api/types'
import { weekOf } from '@/components/day/week'

type HistoryDay = components['schemas']['HistoryDayBody']

// measured tells a day of the history with a measurement.
function measured(d: HistoryDay): boolean {
  return [
    d.weight_kg,
    d.body_fat_pct,
    d.lean_mass_kg,
    d.steps,
    d.active_kcal,
    d.sleep_h,
    d.resting_hr,
    d.hrv,
  ].some((v) => v !== undefined)
}

// useNothingMeasured tells, of today, whether the user has measured
// nothing yet: today has no measurement, no day of the week up to it has
// one (its history, the week strip's), no weigh-in came before (the
// summary has no trend) and no night's sleep, resting heart rate or HRV
// of the last days (recovery). A new user's day then shows no empty
// measurements, only the way to the first. Another day than today, or
// while what tells it comes, or when it did not come, is not: the day
// shows as it always does.
export function useNothingMeasured(date: string, metrics: readonly Metric[]): boolean {
  const me = $api.useQuery('get', '/api/v1/me')
  const today = me.data?.today
  const summary = $api.useQuery('get', '/api/v1/summary')
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query: weekOf(date, today ?? date) } },
    { enabled: today === date },
  )
  if (today !== date || !summary.isSuccess || !history.isSuccess) {
    return false
  }
  const { trend, recovery } = summary.data
  return (
    metrics.length === 0 &&
    !history.data.days.some(measured) &&
    trend === undefined &&
    recovery.sleep_h === undefined &&
    recovery.resting_hr === undefined &&
    recovery.hrv === undefined
  )
}
