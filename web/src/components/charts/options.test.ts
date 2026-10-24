import { expect, test } from 'vitest'

import {
  activityOption,
  average7,
  intervalText,
  macrosOption,
  sleepColor,
  tdeeOption,
  weightOption,
} from '@/components/charts/options'
import { palette } from '@/components/charts/palette'
import { day, svgOf, words } from '@/test/charts'

const days = [
  day('2026-10-20', {
    meals: 2,
    intake: { kcal: 2000, protein: 150, carbs: 200, fat: 60 },
    target: { kcal: 2100, protein: 150, carbs: 220, fat: 60 },
    weight_kg: 82.4,
    trend_kg: 82.6,
    tdee: { kcal: 2400, half_width: 50, trusted: true, before_phase: false, to: '2026-10-20' },
    sleep_h: 7.5,
  }),
  // Nothing that day: gaps, not zeros.
  day('2026-10-21'),
  day('2026-10-22', { trend_kg: 82.5, sleep_h: 5.5 }),
]

// series returns the data of an option's series, by their names.
function series(option: Record<string, unknown>): Record<string, unknown[]> {
  const all: unknown = option.series
  if (!Array.isArray(all)) {
    return {}
  }
  return Object.fromEntries(
    all.flatMap((s: unknown) =>
      typeof s === 'object' && s !== null && 'name' in s && 'data' in s && Array.isArray(s.data)
        ? [[String(s.name), s.data]]
        : [],
    ),
  )
}

test('the weight is its weigh-ins and its trend, with gaps, and the goal as a line', () => {
  const option = weightOption(days, words, 75)
  const s = series(option)
  expect(s['Weigh-in']).toEqual([82.4, null, null])
  expect(s.Trend).toEqual([82.6, null, 82.5])
  expect(JSON.stringify(option)).toContain('"yAxis":75')
  expect(JSON.stringify(weightOption(days, words))).not.toContain('markLine')
  expect(JSON.stringify(option.xAxis)).toContain('["10-20","10-21","10-22"]')
})

test("the macros are each day's kcal of them, and the target", () => {
  const s = series(macrosOption(days, words))
  expect(s.Protein).toEqual([600, null, null])
  expect(s.Carbs).toEqual([800, null, null])
  expect(s.Fat).toEqual([540, null, null])
  expect(s.Target).toEqual([2100, null, null])
})

test('the TDEE is a line in a band of its interval, and the intake as dots', () => {
  const s = series(tdeeOption(days, words))
  expect(s.low).toEqual([2350, null, null])
  expect(s.Interval).toEqual([100, null, null])
  expect(s.TDEE).toEqual([2400, null, null])
  expect(s.Eaten).toEqual([2000, null, null])
})

test('a night is coloured by its length, and the 7 days are averaged', () => {
  expect([7, 6.5, 5.9].map(sleepColor)).toEqual([palette.blue, palette.warn, palette.bad])
  expect(average7([7, null, 5, 8, 8, 8, 8, 8, 2])).toEqual([7, 7, 6, 6.67, 7, 7.2, 7.33, 7.5, 6.71])
  expect(average7([null, null])).toEqual([null, null])
})

test('the goal is drawn though it is far below the weights', () => {
  const svg = svgOf(weightOption(days, words, 75))

  expect(svg).toContain('Goal 75.0')
  expect(svgOf(weightOption(days, words))).not.toContain('Goal')
})

test("a day without a TDEE says no interval in the tooltip, not a number's half", () => {
  expect(intervalText(120, words)).toBe('± 60')
  expect(intervalText(null, words)).toBe('-')
  expect(intervalText(Number.NaN, words)).toBe('-')
})

test('one weigh-in at the goal keeps the axis around it', () => {
  const option = weightOption([day('2026-10-20', { weight_kg: 80, trend_kg: 80 })], words, 80)
  const svg = svgOf(option)
  expect(svg).toContain('>81<')
  expect(svg).toContain('>79<')
  expect(svg).not.toContain('>40<')
})

test('one weigh-in without a goal keeps the axis around it too', () => {
  const svg = svgOf(weightOption([day('2026-10-20', { weight_kg: 80, trend_kg: 80 })], words))
  expect(svg).toContain('>81<')
  expect(svg).toContain('>79<')
  expect(svg).not.toContain('>40<')
})

// labels are an SVG's texts of a colour, each with where it is anchored.
function labels(svg: string, fill: string): { x: number; anchor: string; text: string }[] {
  return [
    ...svg.matchAll(
      /<text[^>]*text-anchor="(\w+)"[^>]*translate\(([\d.]+) [\d.]+\)" fill="([^"]+)">([^<]*)</g,
    ),
  ].flatMap(([, anchor, x, f, text]) =>
    f === fill && anchor !== undefined && x !== undefined && text !== undefined
      ? [{ x: Number(x), anchor, text }]
      : [],
  )
}

test('the labels of both axes fit in the chart, however wide their numbers', () => {
  const svg = svgOf(
    activityOption(
      [
        day('2026-10-20', { steps: 25000, active_kcal: 1500 }),
        day('2026-10-21', { steps: 3000, active_kcal: 300 }),
      ],
      words,
    ),
  )
  // 12px digits are about 7px wide: the steps' labels end far enough in
  // from the left for theirs, and the active energy's start far enough
  // in from the right.
  const steps = labels(svg, palette.muted).filter((l) => l.anchor === 'end')
  const active = labels(svg, palette.accent)
  expect(steps.map((l) => l.text)).toContain('25,000')
  expect(active.map((l) => l.text)).toContain('1,500')
  for (const l of steps) {
    expect(l.x - l.text.length * 6).toBeGreaterThanOrEqual(0)
  }
  for (const l of active) {
    expect(l.x + l.text.length * 6).toBeLessThanOrEqual(360)
  }
})
