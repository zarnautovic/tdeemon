import { renderHook } from '@testing-library/react'
import { expect, test } from 'vitest'

import { parseAmount } from '@/components/day/meal-entries'
import { recipeQuantity, recipeUnit, shareField, shareOf } from '@/components/recipes/share'
import { en } from '@/i18n/en'
import { useFormat } from '@/i18n/format'
import { hr } from '@/i18n/hr'
import { setLocale } from '@/i18n/locale'

// The app's formats, in English.
const f = renderHook(() => useFormat()).result.current

test('a share of a recipe is the nearest small fraction a person writes', () => {
  expect(shareOf(0.25, f.number)).toBe('¼')
  expect(shareOf(0.1, f.number)).toBe('⅒')
  expect(shareOf(1 / 3, f.number)).toBe('⅓')
  expect(shareOf(2 / 3, f.number)).toBe('⅔')
  // Of no glyph of its own.
  expect(shareOf(1 / 12, f.number)).toBe('1/12')
  expect(shareOf(5 / 12, f.number)).toBe('5/12')
  // Whole dishes before it.
  expect(shareOf(1.5, f.number)).toBe('1½')
  // Near enough: 0.33 is a third, as a coach may have sent it.
  expect(shareOf(0.33, f.number)).toBe('⅓')
  // A twelfth is the smallest: a fourteenth is a number.
  expect(shareOf(1 / 14, f.number)).toBe('0.07')
  expect(shareOf(0.37, f.number)).toBe('0.37')
})

test('a share starts its field as a fraction, read back as it was', () => {
  expect(shareField(1 / 3, f.field)).toBe('1/3')
  expect(shareField(0.1, f.field)).toBe('1/10')
  expect(shareField(1.5, f.field)).toBe('3/2')
  expect(shareField(1, f.field)).toBe('1')
  expect(shareField(0.37, f.field)).toBe('0.37')
  // Every decimal: a third typed back is a third.
  expect(parseAmount(shareField(1 / 3, f.field), f.parse)).toBe(1 / 3)
})

test('an amount is typed as a number or a fraction', () => {
  expect(parseAmount('1/10', f.parse)).toBe(0.1)
  expect(parseAmount('2/3', f.parse)).toBe(2 / 3)
  expect(parseAmount(' 1 / 4 ', f.parse)).toBe(0.25)
  expect(parseAmount('1 1/2', f.parse)).toBe(1.5)
  expect(parseAmount('1.5/3', f.parse)).toBe(0.5)
  expect(parseAmount('0.25', f.parse)).toBe(0.25)
  expect(parseAmount('1/', f.parse)).toBeUndefined()
  expect(parseAmount('1/0', f.parse)).toBeUndefined()
  expect(parseAmount('1 1/0', f.parse)).toBeUndefined()
  expect(parseAmount('a/b', f.parse)).toBeUndefined()
  expect(parseAmount('1/2/3', f.parse)).toBeUndefined()
})

test("a recipe's quantity is a share of it or portions, in each language", () => {
  const recipe = (servings: number, serving: string) =>
    recipeQuantity('recipe', { servings, serving }, en, f.number)
  expect(recipe(0.1, 'recipe')).toBe('⅒ of the recipe')
  expect(recipe(1, 'recipe')).toBe('1 recipe')
  expect(recipe(2, 'recipe')).toBe('2 recipes')
  expect(recipe(1, 'portion')).toBe('1 portion')
  expect(recipe(0.999, 'portion')).toBe('1 portion')
  expect(recipe(2.5, 'portion')).toBe('2.5 portions')
  // Another food's serving, or no serving, is not a recipe's.
  expect(recipeQuantity('label', { servings: 1, serving: 'recipe' }, en, f.number)).toBeUndefined()
  expect(recipe(1, 'cup')).toBeUndefined()
  expect(recipeQuantity('recipe', { serving: 'recipe' }, en, f.number)).toBeUndefined()

  setLocale('hr')
  const hrf = renderHook(() => useFormat()).result.current
  const recept = (servings: number, serving: string) =>
    recipeQuantity('recipe', { servings, serving }, hr, hrf.number)
  expect(recept(0.1, 'recipe')).toBe('⅒ recepta')
  expect(recept(1, 'portion')).toBe('1 porcija')
  expect(recept(2, 'portion')).toBe('2 porcije')
  expect(recept(5, 'portion')).toBe('5 porcija')
  expect(recept(1.5, 'portion')).toBe('1,5 porcija')
  setLocale('en')
})

test("a recipe's serving as a unit", () => {
  expect(recipeUnit('recipe', 'recipe', 0.25, en)).toBe('of the recipe')
  expect(recipeUnit('recipe', 'portion', 2, en)).toBe('portions')
  expect(recipeUnit('recipe', 'cup', 1, en)).toBeUndefined()
  expect(recipeUnit('label', 'recipe', 1, en)).toBeUndefined()
  expect(recipeUnit('recipe', 'recipe', 0.25, hr)).toBe('recepta')
})
