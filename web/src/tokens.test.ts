import { expect, test } from 'vitest'

import { themeTokens } from '@/test/charts'

// The colours that mean something, and the macros', which only name a
// macro: one of the first is never one of the second, nor near it, or a
// day over its target would look like fat, and a warning like carbs.
const meaning = ['--primary', '--warning', '--destructive', '--over'] as const
const macros = ['--protein', '--carbs', '--fat'] as const

// apart is how far two colours must be, in OKLab (0 to 1 from black to
// white): a tenth, five times what an eye just tells apart.
const apart = 0.1

// resolved is a token's value in a theme, following the tokens it names.
function resolved(theme: Record<string, string>, name: string): string {
  const value = theme[name] ?? ''
  const named = /^var\((--[\w-]+)\)$/.exec(value)?.[1]
  return named === undefined ? value : resolved(theme, named)
}

// linear is a colour's channels, #rrggbb, as light, 0 to 1.
function linear(hex: string): [number, number, number] {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = Number.parseInt(hex.slice(i, i + 2), 16) / 255
    return c <= 0.040_45 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return [r ?? 0, g ?? 0, b ?? 0]
}

// oklab is a colour in OKLab (Björn Ottosson's), where a distance is
// what an eye sees.
function oklab(hex: string): [number, number, number] {
  const [r, g, b] = linear(hex)
  const l = Math.cbrt(0.412_221_470_8 * r + 0.536_332_536_3 * g + 0.051_445_992_9 * b)
  const m = Math.cbrt(0.211_903_498_2 * r + 0.680_699_545_1 * g + 0.107_396_956_6 * b)
  const s = Math.cbrt(0.088_302_461_9 * r + 0.281_718_837_6 * g + 0.629_978_700_5 * b)
  return [
    0.210_454_255_3 * l + 0.793_617_785 * m - 0.004_072_046_8 * s,
    1.977_998_495_1 * l - 2.428_592_205 * m + 0.450_593_709_9 * s,
    0.025_904_037_1 * l + 0.782_771_766_2 * m - 0.808_675_766 * s,
  ]
}

// distance is how far apart two colours are, in OKLab.
function distance(one: string, other: string): number {
  const [p, q] = [oklab(one), oklab(other)]
  return Math.hypot(p[0] - q[0], p[1] - q[1], p[2] - q[2])
}

// luminance is a colour's relative luminance, as WCAG has it.
function luminance(hex: string): number {
  const [r, g, b] = linear(hex)
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

// contrast is WCAG's contrast ratio of two colours.
function contrast(one: string, other: string): number {
  const [hi, lo] = [luminance(one), luminance(other)].toSorted((x, y) => y - x)
  return ((hi ?? 0) + 0.05) / ((lo ?? 0) + 0.05)
}

const themes = Object.entries(themeTokens())

test.each(themes)(
  'in the %s theme, no colour that means something is near a macro’s',
  (_, theme) => {
    const near = meaning.flatMap((m) =>
      macros
        .map((macro) => [m, macro, distance(resolved(theme, m), resolved(theme, macro))] as const)
        .filter(([, , d]) => d < apart),
    )
    expect(near).toEqual([])
  },
)

test.each(themes)(
  'in the %s theme, over is none of the other colours that mean something',
  (_, theme) => {
    const near = meaning
      .filter((m) => m !== '--over')
      .map((m) => [m, distance(resolved(theme, '--over'), resolved(theme, m))] as const)
      .filter(([, d]) => d < apart)
    expect(near).toEqual([])
  },
)

// Over draws an arc, a bar's part and a large number: 3:1 on what is
// behind them.
test.each(themes)(
  'in the %s theme, over keeps 3:1 on the page, a card and surface-2',
  (_, theme) => {
    const over = resolved(theme, '--over')
    const low = ['--background', '--card', '--surface-2']
      .map((s) => [s, contrast(over, resolved(theme, s))] as const)
      .filter(([, c]) => c < 3)
    expect(low).toEqual([])
  },
)

// The test's own measure, on colours whose distance is known: the dark
// theme's carbs and warning were one colour before they were told apart.
test('the measure tells one colour from another, and the same from itself', () => {
  expect(distance('#f0b45c', '#f0b45c')).toBe(0)
  expect(distance('#000000', '#ffffff')).toBeCloseTo(1, 3)
  expect(contrast('#000000', '#ffffff')).toBeCloseTo(21, 3)
})
