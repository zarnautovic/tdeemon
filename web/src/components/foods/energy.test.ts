import { expect, test } from 'vitest'

import { energyOff, kcalOf } from '@/components/foods/energy'

test('the energy of the values is by the factors of an EU label', () => {
  expect(kcalOf({ protein: 20, carbs: 5, fat: 1.5 })).toBe(113.5)
  // 10 g of the carbs polyols, 2.4 kcal each; fibre 2; alcohol 7.
  expect(kcalOf({ protein: 0, carbs: 30, fat: 0, polyols: 10 })).toBe(104)
  expect(kcalOf({ protein: 0, carbs: 0, fat: 0, fiber: 10, alcohol: 10 })).toBe(90)
  // More polyols than carbs, which hold them, count as the carbs.
  expect(kcalOf({ protein: 0, carbs: 5, fat: 0, polyols: 8 })).toBe(12)
})

test('the energy said far from the values is told, with what they make', () => {
  // 300 kcal of P 20 / C 5 / F 1.5, which make about 113.
  expect(energyOff({ kcal: 300, protein: 20, carbs: 5, fat: 1.5 })).toBe(113.5)
  // Too little as much as too much: 50 kcal of what makes 375.
  expect(energyOff({ kcal: 50, protein: 13, carbs: 60, fat: 7, fiber: 10 })).toBe(375)
  // Energy of no macros at all.
  expect(energyOff({ kcal: 120, protein: 0, carbs: 0, fat: 0 })).toBe(0)
})

test("a label's rounding, and a few kcal, are no warning", () => {
  // Oats: 370 kcal, the values make 375.
  expect(energyOff({ kcal: 370, protein: 13, carbs: 60, fat: 7, fiber: 10 })).toBeUndefined()
  // An apple's 52 kcal of what makes 63.8: 19 % off, within a fifth.
  expect(energyOff({ kcal: 52, protein: 0.3, carbs: 14, fat: 0.2, fiber: 2.4 })).toBeUndefined()
  // A drink of 4 kcal its values make 0 of: far in share, near in kcal.
  expect(energyOff({ kcal: 4, protein: 0, carbs: 0, fat: 0 })).toBeUndefined()
  expect(energyOff({ kcal: 0, protein: 0, carbs: 0, fat: 0 })).toBeUndefined()
  // Sugar-free gum: its polyols' 2.4 kcal a gram.
  expect(energyOff({ kcal: 155, protein: 0, carbs: 64, fat: 0, polyols: 63 })).toBeUndefined()
})
