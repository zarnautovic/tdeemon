import type { Messages } from '@/i18n/en'

// The servings of a recipe's food, by their names (package recipe): the
// whole dish, and one of its portions when it is divided.
export const wholeServing = 'recipe'
export const portionServing = 'portion'

// The largest denominator a share of a recipe is written with: a twelfth
// of a pot is as small as anyone divides one by eye.
const mostDenominator = 12

// fractionNear is x as a fraction n/d of a denominator up to 12, the
// nearest within tolerance (a share of x, of 1 % of it read, of a hair
// typed), the smaller denominator of two as near; undefined when none is.
function fractionNear(x: number, tolerance: number): [number, number] | undefined {
  let best: [number, number] | undefined
  let off = Number.POSITIVE_INFINITY
  for (let d = 1; d <= mostDenominator; d++) {
    const n = Math.round(x * d)
    const by = Math.abs(n / d - x)
    if (n > 0 && by <= tolerance && by < off - 1e-12) {
      best = [n, d]
      off = by
    }
  }
  return best
}

// The fractions of their own glyph, by n/d.
const glyphs: Record<string, string> = {
  '1/2': '½',
  '1/3': '⅓',
  '2/3': '⅔',
  '1/4': '¼',
  '3/4': '¾',
  '1/5': '⅕',
  '2/5': '⅖',
  '3/5': '⅗',
  '4/5': '⅘',
  '1/6': '⅙',
  '5/6': '⅚',
  '1/7': '⅐',
  '1/8': '⅛',
  '3/8': '⅜',
  '5/8': '⅝',
  '7/8': '⅞',
  '1/9': '⅑',
  '1/10': '⅒',
}

// shareOf is a share of a whole dish as a person writes it: the nearest
// fraction of a denominator up to 12 within 1.2 % of it (0.33 is ⅓),
// small (⅒), its whole dishes before it (1½), or n/d where no glyph is
// (5/12, and 1 5/12 apart from its whole dishes); else the number
// (number, 0.07), two digits of it that are not 0 when it is that small
// (0.004 of a big pot).
export function shareOf(x: number, number: (n: number, digits?: number) => string): string {
  const near = fractionNear(x, x * 0.012)
  if (near === undefined) {
    return small(x, number)
  }
  const [n, d] = near
  const whole = Math.floor(n / d)
  const rest = n % d
  if (rest === 0) {
    return number(whole)
  }
  const glyph = glyphs[`${String(rest)}/${String(d)}`]
  const part = glyph ?? `${String(rest)}/${String(d)}`
  if (whole === 0) {
    return part
  }
  // A glyph sits by its whole dishes (1½); digits keep a space from them,
  // which would read as one number (15/12).
  return glyph === undefined ? `${number(whole)} ${part}` : `${number(whole)}${part}`
}

// small is a number of two decimals, or of two digits that are not 0
// when it is smaller than that: 0.004 is not 0.
function small(x: number, number: (n: number, digits?: number) => string): string {
  return x > 0 && x < 0.01 ? number(Number(x.toPrecision(2)), 10) : number(x, 2)
}

// shareField is a share of a dish as its field starts with it, to be read
// back as it was (parseAmount): a fraction it is (1/3, 3/2), else the
// number as field writes it, to three decimals, or two digits that are
// not 0 when it is smaller (small).
export function shareField(x: number, field: (n: number, digits?: number) => string): string {
  const near = fractionNear(x, 1e-9 * Math.max(1, x))
  if (near === undefined || near[1] === 1) {
    return x > 0 && x < 0.01 ? field(Number(x.toPrecision(2)), 10) : field(x)
  }
  return `${String(near[0])}/${String(near[1])}`
}

// recipeQuantity is how much of a recipe was given as its own servings
// say it: a share of the whole dish ("⅒ of the recipe", or "2 recipes"
// whole), or portions ("2 portions", in the language's plural); undefined
// for another food's serving, or another quantity. The share and the
// portions are exact, whether the dish was weighed or not.
export function recipeQuantity(
  source: string | undefined,
  quantity: { servings?: number | undefined; serving?: string | undefined },
  t: Messages,
  number: (n: number, digits?: number) => string,
): string | undefined {
  const { servings, serving } = quantity
  if (source !== 'recipe' || servings === undefined) {
    return undefined
  }
  if (serving === wholeServing) {
    return Number.isInteger(servings)
      ? `${number(servings)} ${t.recipes.servings.recipe(servings)}`
      : t.recipes.ofRecipe(shareOf(servings, number))
  }
  if (serving === portionServing) {
    // The word's form follows the number shown: 0.999 is "1 portion".
    const shown = Math.round(servings * 100) / 100
    return t.recipes.portions(number(shown, 2), shown)
  }
  return undefined
}

// recipeUnit is a recipe's serving as a unit beside its amount: "of the
// recipe" after a share typed, "portions" after their count; undefined
// for another food's serving.
export function recipeUnit(
  source: string | undefined,
  serving: string,
  n: number,
  t: Messages,
): string | undefined {
  if (source !== 'recipe') {
    return undefined
  }
  if (serving === wholeServing) {
    return t.recipes.ofTheRecipe
  }
  return serving === portionServing ? t.recipes.servings.portion(n) : undefined
}
