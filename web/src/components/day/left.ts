import { rounded, roundedGrams } from '@/i18n/format'

// What a day's target leaves is the target as it is shown less what was
// eaten as it is shown, so the two add up to the target on the screen
// (as far as the grams' rounding lets them), and the ring, the macros,
// the meals' last row and the meal being logged say the same. Below 0
// past the target.

// kcalLeft is what a target of kcal leaves after eaten, both whole.
export function kcalLeft(eaten: number, target: number): number {
  return rounded(target) - rounded(eaten)
}

// gramsLeft is what a target of grams leaves after eaten, each as grams
// are shown (roundedGrams); to the tenth, as 5 less 3.7 is not quite 1.3
// in floating point.
export function gramsLeft(eaten: number, target: number): number {
  return rounded(roundedGrams(target) - roundedGrams(eaten), 1)
}

// Standing is where a day's energy is against its target: under it, on
// it, or over it.
export type Standing = 'under' | 'on' | 'over'

// kcalStanding is where eaten stands against a target of kcal, both as
// shown (kcalLeft): on it within 5 % of it either way, and 100 kcal at
// least, as the plan works on the week's average, which such a day
// leaves as it is; under or over it past that.
export function kcalStanding(eaten: number, target: number): Standing {
  const left = kcalLeft(eaten, target)
  if (Math.abs(left) <= Math.max(rounded(target) * 0.05, 100)) {
    return 'on'
  }
  return left > 0 ? 'under' : 'over'
}
