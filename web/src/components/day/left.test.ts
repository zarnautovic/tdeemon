import { expect, test } from 'vitest'

import { kcalStanding } from '@/components/day/left'

// Within 5 % of the target either way, and 100 kcal at least, a day is
// on it: 2,300 of 2,250 is, though 50 over.
test.each([
  [2300, 2250, 'on'],
  [2200, 2250, 'on'],
  // 5 % of 2,250 is 112.5.
  [2362, 2250, 'on'],
  [2363, 2250, 'over'],
  [2137, 2250, 'under'],
  // A small target is on within 100 kcal, more than its 5 %.
  [1300, 1200, 'on'],
  [1301, 1200, 'over'],
  [1099, 1200, 'under'],
  // As shown: 1,300.4 is 1,300, 100 over 1,200.
  [1300.4, 1200, 'on'],
  [2930, 2250, 'over'],
] as const)('%d of %d kcal is %s its target', (eaten, target, standing) => {
  expect(kcalStanding(eaten, target)).toBe(standing)
})
