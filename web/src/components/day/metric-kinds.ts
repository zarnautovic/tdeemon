import type { Metric } from '@/api/types'

export type MetricKind = Metric['kind']

// metricKinds are the kinds of measurement, in the server's order.
export const metricKinds: readonly MetricKind[] = [
  'weight',
  'body_fat',
  'lean_mass',
  'steps',
  'active_kcal',
  'basal_kcal',
  'sleep_h',
  'rhr',
  'hrv',
  'vo2max',
  'exercise_min',
]

// units are the units a value of each kind is given in, the server's
// symbols (metric.specs); steps are a count, without one.
export const units: Record<MetricKind, string> = {
  weight: 'kg',
  body_fat: '%',
  lean_mass: 'kg',
  steps: '',
  active_kcal: 'kcal',
  basal_kcal: 'kcal',
  sleep_h: 'h',
  rhr: 'bpm',
  hrv: 'ms',
  vo2max: 'ml/kg/min',
  exercise_min: 'min',
}

// digits are the decimals a kind of measurement is shown with.
export const digits: Partial<Record<MetricKind, number>> = {
  weight: 1,
  body_fat: 1,
  lean_mass: 1,
  sleep_h: 1,
  vo2max: 1,
}
