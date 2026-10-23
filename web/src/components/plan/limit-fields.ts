import type { components } from '@/api/schema'

export type LimitsBody = components['schemas']['LimitsBody']

// The units a limit is shown in.
export type Unit =
  | 'pctWeek'
  | 'kgWeek'
  | 'gPerKg'
  | 'g'
  | 'kcal'
  | 'pctKcal'
  | 'pctAvg'
  | 'pctTdee'
  | 'days'
  | 'ofWeek'

export type Group = 'rates' | 'food' | 'autopilot'

export type LimitName =
  | 'cut_rate'
  | 'cut_rate_obese'
  | 'loss_kg_week'
  | 'bulk_rate'
  | 'recomp_min'
  | 'recomp_max'
  | 'protein_floor'
  | 'protein_ceiling'
  | 'protein_floor_grams'
  | 'kcal_floor_male'
  | 'kcal_floor_female'
  | 'fat_floor_pct'
  | 'step_kcal'
  | 'step_pct'
  | 'interval_days'
  | 'first_step_days'
  | 'logged_days'
  | 'weigh_ins'
  | 'drift_pct'

// A LimitField is how the page shows a limit of docs/limits.md ("The
// names of the limits"). A loss is shown as how fast, a positive number,
// and stored as the rate, below 0; a signed one may be either.
export interface LimitField {
  name: LimitName
  group: Group
  unit: Unit
  loss?: boolean
  signed?: boolean
  whole?: boolean
}

export const limitFields: readonly LimitField[] = [
  { name: 'cut_rate', group: 'rates', unit: 'pctWeek', loss: true },
  { name: 'cut_rate_obese', group: 'rates', unit: 'pctWeek', loss: true },
  { name: 'loss_kg_week', group: 'rates', unit: 'kgWeek' },
  { name: 'bulk_rate', group: 'rates', unit: 'pctWeek' },
  { name: 'recomp_min', group: 'rates', unit: 'pctWeek', signed: true },
  { name: 'recomp_max', group: 'rates', unit: 'pctWeek', signed: true },
  { name: 'protein_floor', group: 'food', unit: 'gPerKg' },
  { name: 'protein_ceiling', group: 'food', unit: 'gPerKg' },
  { name: 'protein_floor_grams', group: 'food', unit: 'g' },
  { name: 'kcal_floor_male', group: 'food', unit: 'kcal' },
  { name: 'kcal_floor_female', group: 'food', unit: 'kcal' },
  { name: 'fat_floor_pct', group: 'food', unit: 'pctKcal' },
  { name: 'step_kcal', group: 'autopilot', unit: 'kcal' },
  { name: 'step_pct', group: 'autopilot', unit: 'pctAvg' },
  { name: 'interval_days', group: 'autopilot', unit: 'days', whole: true },
  { name: 'first_step_days', group: 'autopilot', unit: 'days', whole: true },
  { name: 'logged_days', group: 'autopilot', unit: 'ofWeek', whole: true },
  { name: 'weigh_ins', group: 'autopilot', unit: 'ofWeek', whole: true },
  { name: 'drift_pct', group: 'autopilot', unit: 'pctTdee' },
]

// The limits that bound one another, a minimum and its maximum, which
// the server refuses the other way round.
const pairs = [
  ['recomp_min', 'recomp_max'],
  ['protein_floor', 'protein_ceiling'],
] as const

// shown is a stored value as the field shows it.
export function shown(field: LimitField, v: number): number {
  return field.loss && v !== 0 ? -v : v
}

// typedValue is the stored value of what was typed in a field, or
// undefined for no number: a loss is typed as how fast, a signed value
// with a minus or without.
export function typedValue(
  field: LimitField,
  typed: string,
  parse: (s: string) => number | undefined,
): number | undefined {
  const s = typed.trim()
  if (field.signed && /^[-−]/.test(s)) {
    const v = parse(s.slice(1))
    return v === undefined || v === 0 ? v : -v
  }
  const v = parse(s)
  return v === undefined ? undefined : shown(field, v)
}

// looser tells whether a is a looser value of the limit than b.
export function looser(body: LimitsBody, name: string, a: number, b: number): boolean {
  return body.looser_below.includes(name) ? a < b : a > b
}

// A Problem is why a value of a limit cannot be sent as it is.
export type Problem =
  | { kind: 'number' }
  | { kind: 'whole' }
  // past the instance's limit, which is value
  | { kind: 'instance'; value: number }
  // a minimum past its maximum, or the other way round, both the user's
  | { kind: 'order'; other: LimitName }
  // past the instance's other, which holds over the user's: to be
  // lowered to value at most (a minimum), or raised to it at least (a
  // maximum)
  | { kind: 'held'; other: LimitName; value: number; to: 'lower' | 'raise' }

// A Check is what the typed limits come to: the values that change, and
// those that loosen a limit, by name, or why some cannot be sent.
export interface Check {
  changes: Partial<Record<LimitName, number>>
  loosened: LimitName[]
  problems: Partial<Record<LimitName, Problem>>
}

// check reads the typed values of the limits (by name; those not typed
// stay) against the limits as they are, as the server does: a number of
// the right kind, no further than the instance's, a minimum within its
// maximum, the instance's included.
export function check(
  body: LimitsBody,
  typed: Readonly<Record<string, string>>,
  parse: (s: string) => number | undefined,
): Check {
  const changes: Partial<Record<LimitName, number>> = {}
  const problems: Partial<Record<LimitName, Problem>> = {}
  const next: Record<string, number> = { ...body.limits }
  for (const field of limitFields) {
    const s = typed[field.name]
    if (s === undefined) {
      continue
    }
    const v = typedValue(field, s, parse)
    if (v === undefined) {
      problems[field.name] = { kind: 'number' }
      continue
    }
    if (field.whole && !Number.isInteger(v)) {
      problems[field.name] = { kind: 'whole' }
      continue
    }
    if (v === body.limits[field.name]) {
      continue
    }
    next[field.name] = v
    changes[field.name] = v
    const instance = body.instance[field.name]
    if (instance !== undefined && looser(body, field.name, v, instance)) {
      problems[field.name] = { kind: 'instance', value: instance }
    }
  }
  for (const [min, max] of pairs) {
    const lo = next[min]
    const hi = next[max]
    if (lo === undefined || hi === undefined || problems[min] || problems[max]) {
      continue
    }
    if (lo > hi) {
      const changed = min in changes ? min : max
      problems[changed] = { kind: 'order', other: changed === min ? max : min }
      continue
    }
    // The instance's hold over the user's looser ones, so a minimum past
    // the instance's maximum, which it may have lowered since, is refused
    // with any change: the user's is the one to move.
    const instLo = body.instance[min]
    const instHi = body.instance[max]
    if (instHi !== undefined && lo > instHi) {
      problems[min] = { kind: 'held', other: max, value: instHi, to: 'lower' }
    } else if (instLo !== undefined && hi < instLo) {
      problems[max] = { kind: 'held', other: min, value: instLo, to: 'raise' }
    }
  }
  const loosened = limitFields
    .map((field) => field.name)
    .filter((name) => {
      const v = changes[name]
      const current = body.limits[name]
      return v !== undefined && current !== undefined && looser(body, name, v, current)
    })
  return { changes, loosened, problems }
}
