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
