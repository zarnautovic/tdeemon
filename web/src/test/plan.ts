import type { components } from '@/api/schema'
import type { LimitsBody } from '@/components/plan/limit-fields'

// planBody is a plan of nothing yet, as GET /api/v1/plan answers it; over
// is laid over it.
export function planBody(
  over: Partial<components['schemas']['PlanBody']> = {},
): components['schemas']['PlanBody'] {
  return {
    today: '2026-10-23',
    phases: [],
    training_weekdays: [],
    targets: [],
    limits: limitsBody().effective,
    revisions: [],
    autopilot: { mode: 'propose', next: '2026-10-26', drift_kcal: 0 },
    ...over,
  }
}

// autopilotBody is the autopilot of a user on a cut, asking first, its
// review on Mondays and none yet, as GET /api/v1/autopilot answers it;
// over is laid over it.
export function autopilotBody(
  over: Partial<components['schemas']['AutopilotBody']> = {},
): components['schemas']['AutopilotBody'] {
  return {
    mode: 'propose',
    weekday: 'monday',
    next: '2026-10-26',
    step_kcal: 100,
    drift: { used_kcal: 0, limit_pct: 15, agreed_on: '2026-09-28', agreed_kcal: 2300 },
    ...over,
  }
}

// review is a weekly review of today's, a step of 100 kcal down on a cut
// too slow, applied; over is laid over it.
export function review(
  over: Partial<components['schemas']['ReviewBody']> = {},
): components['schemas']['ReviewBody'] {
  return {
    date: '2026-10-23',
    outcome: 'applied',
    reason: 'cut_slow',
    step_kcal: -100,
    gap_kcal: -180,
    past_drift: false,
    measured_pct_week: -0.31,
    target_pct_week: -0.5,
    tolerance_pct_week: 0.1,
    tdee_kcal: 2600,
    tdee_half_width: 120,
    intake_kcal: 2140,
    target_kcal: 2179,
    after_kcal: 2079,
    logged_days: 6,
    weigh_ins: 5,
    needs_logged_days: 4,
    needs_weigh_ins: 3,
    steps_change_pct: -3,
    carbs_g: 0,
    carbs_floor_g: 130,
    bmi: 25.9,
    drift_kcal: -200,
    drift_limit_kcal: 390,
    ...over,
  }
}

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
