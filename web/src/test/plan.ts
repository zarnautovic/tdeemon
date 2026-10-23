import type { LimitsBody } from '@/components/plan/limit-fields'

// limitsBody is the limits of a user at the start, on an instance's
// defaults, with over laid over the user's.
export function limitsBody(over: Record<string, number> = {}): LimitsBody {
  const defaults = {
    cut_rate: -0.7,
    cut_rate_obese: -1,
    loss_kg_week: 1.5,
    bulk_rate: 0.25,
    recomp_min: -0.5,
    recomp_max: 0.25,
    protein_floor: 1.6,
    protein_ceiling: 3,
    protein_floor_grams: 0,
    kcal_floor_male: 1500,
    kcal_floor_female: 1200,
    fat_floor_pct: 20,
    step_kcal: 100,
    step_pct: 5,
    interval_days: 7,
    first_step_days: 14,
    logged_days: 4,
    weigh_ins: 3,
    drift_pct: 15,
  }
  const { protein_floor_grams: _, ...rest } = defaults
  const instance = {
    ...rest,
    cut_rate: -1,
    cut_rate_obese: -1.25,
    bulk_rate: 0.5,
    protein_floor: 0.8,
    kcal_floor_male: 1200,
    fat_floor_pct: 15,
    step_kcal: 200,
    step_pct: 10,
    drift_pct: 25,
  }
  const limits = { ...defaults, ...over }
  return {
    limits,
    instance,
    effective: limits,
    defaults,
    looser_below: [
      'cut_rate',
      'cut_rate_obese',
      'recomp_min',
      'protein_floor',
      'protein_floor_grams',
      'kcal_floor_male',
      'kcal_floor_female',
      'fat_floor_pct',
      'interval_days',
      'first_step_days',
      'logged_days',
      'weigh_ins',
    ],
  }
}
