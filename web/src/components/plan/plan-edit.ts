import type { LimitName } from '@/components/plan/limit-fields'
import { useLimitWords, type Violation } from '@/components/plan/proposal-text'
import { useMessages } from '@/i18n/messages'

// The fields of a target a form edits, of each type of day.
export type DayType = 'training' | 'rest'
export type TargetField = 'kcal' | 'protein' | 'fat'

// The fields of a phase a form edits whose value a limit holds.
export type PhaseField = 'rate' | 'goal' | 'milestone'

// targetFields are the rules of the limits a target's value breaks, by
// the field that holds it. The others (the rate it implies, the BMI) are
// the target's as a whole.
const targetFields: Partial<Record<string, TargetField>> = {
  kcal_floor: 'kcal',
  protein_floor: 'protein',
  protein_ceiling: 'protein',
  protein_floor_grams: 'protein',
  fat_floor: 'fat',
}

// targetFieldOf is the field a violation of the limits is said at, and
// the types of day: one, or both for a target the same on both.
export function targetFieldOf(
  v: Violation,
): { field: TargetField; days: readonly DayType[] } | undefined {
  const field = targetFields[v.rule]
  if (field === undefined) {
    return undefined
  }
  return {
    field,
    days: v.day_type === 'training' || v.day_type === 'rest' ? [v.day_type] : ['training', 'rest'],
  }
}

const phaseFields: Partial<Record<string, PhaseField>> = {
  cut_rate: 'rate',
  bulk_rate: 'rate',
  recomp_rate: 'rate',
  loss_kg_week: 'rate',
  underweight_loss: 'rate',
  underweight_goal: 'goal',
  underweight_milestone: 'milestone',
}

// phaseFieldOf is the field of a phase a violation of the limits is said
// at; undefined for one of the phase as a whole.
export function phaseFieldOf(v: Violation): PhaseField | undefined {
  return phaseFields[v.rule]
}

// limitOf is the user's limit whose unit a rule's values are in, and
// whether it is a loss, said as how fast.
const limitOf: Partial<Record<string, { name: LimitName; loss?: boolean }>> = {
  kcal_floor: { name: 'kcal_floor_male' },
  fat_floor: { name: 'fat_floor_pct' },
  protein_floor: { name: 'protein_floor' },
  protein_ceiling: { name: 'protein_ceiling' },
  protein_floor_grams: { name: 'protein_floor_grams' },
  cut_rate: { name: 'cut_rate', loss: true },
  loss_kg_week: { name: 'loss_kg_week', loss: true },
  bulk_rate: { name: 'bulk_rate' },
  recomp_rate: { name: 'recomp_min' },
}

// useViolationAt says a violation of the limits at the field it is of:
// how far the value may go, in its unit, and whose limit that is ("At
// least 1,500 kcal: your limit"); one with no value of its own, by its
// rule ("a goal below a BMI of 18.5").
export function useViolationAt(): (v: Violation) => string {
  const t = useMessages()
  const limit = useLimitWords()
  const e = t.plan.edit
  const isLevel = (l: string): l is keyof typeof e.levels => Object.hasOwn(e.levels, l)
  const isRule = (r: string): r is keyof typeof t.plan.rules => Object.hasOwn(t.plan.rules, r)
  return (v) => {
    const level = v.level !== undefined && isLevel(v.level) ? e.levels[v.level] : undefined
    const of = limitOf[v.rule]
    let bound: string
    if (of === undefined) {
      bound = isRule(v.rule) ? t.plan.rules[v.rule] : v.rule
    } else {
      // A loss is said as how fast: at most so fast, whatever its sign.
      const value = limit.value(of.name, of.name === 'loss_kg_week' ? Math.abs(v.limit) : v.limit)
      bound = of.loss === true || v.proposed > v.limit ? e.atMost(value) : e.atLeast(value)
    }
    const said = level === undefined ? bound : `${bound}: ${level}`
    return `${said.charAt(0).toUpperCase()}${said.slice(1)}.`
  }
}
