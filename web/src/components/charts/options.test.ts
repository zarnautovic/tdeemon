import { act, renderHook } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import {
  average7,
  eaten7,
  goalShown,
  lineOption,
  nightOf,
  reachOf,
  sleepOption,
  stepsOf,
  stepsOption,
  tdeeOption,
  weekly,
  weightOption,
} from '@/components/charts/options'
import { palette } from '@/components/charts/palette'
import { useChartWords } from '@/components/charts/words'
import { setLocale } from '@/i18n/locale'
import { axisLabelOf, day, labelledOn, svgOf, tooltipOf, words } from '@/test/charts'

afterEach(() => {
  act(() => {
    setLocale('en')
  })
})

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

test('the weight is its weigh-ins and its trend, with gaps, and a goal near them as a line', () => {
  const option = weightOption(days, words, palette, 82.5)
  const s = series(option)
  expect(s['Weigh-in']).toEqual([82.4, null, null])
  expect(s.Trend).toEqual([82.6, null, 82.5])
  expect(JSON.stringify(option)).toContain('"yAxis":82.5')
  expect(JSON.stringify(weightOption(days, words, palette))).not.toContain('markLine')
  expect(JSON.stringify(option.xAxis)).toContain('["2026-10-20","2026-10-21","2026-10-22"]')
  expect(axisLabelOf(option, 'xAxis', '2026-10-20')).toBe('10-20')
})

test("a day's tooltip says its date and values in a line", () => {
  const option = weightOption(days, words, palette)
  expect(tooltipOf(option, 0)).toBe('2026-10-20 · 82.4 kg · trend 82.6')
  expect(tooltipOf(option, 1)).toBe('2026-10-21')
  expect(tooltipOf(option, 2)).toBe('2026-10-22 · trend 82.5')
  expect(tooltipOf(tdeeOption(days, words, palette), 0)).toBe(
    '2026-10-20 · TDEE 2400 ± 50 · eaten 2000 · target 2100',
  )
})

test('the TDEE is a line in a band of its interval, what was eaten dots, the target dashed', () => {
  const option = tdeeOption(days, words, palette)
  const s = series(option)
  expect(s.low).toEqual([2350, null, null])
  expect(s['80% interval']).toEqual([100, null, null])
  expect(s.TDEE).toEqual([2400, null, null])
  expect(s.Eaten).toEqual([2000, null, null])
  expect(s['Target, 7-day average']).toEqual([2100, 2100, 2100])
  // No legend of ECharts': the card has its own, which wraps.
  expect(option.legend).toBeUndefined()
})

test("the target is its 7 days' average, a training day's and a rest day's not a zigzag", () => {
  const targets = [2400, 2000, 2400, 2000].map((kcal, i) =>
    day(`2026-10-2${String(i)}`, { target: { kcal, protein: 150, carbs: 200, fat: 60 } }),
  )
  const option = tdeeOption(targets, words, palette)
  expect(series(option)['Target, 7-day average']).toEqual([2400, 2200, 2266.67, 2200])
  // A tooltip says the day's own.
  expect(tooltipOf(option, 1)).toBe('2026-10-21 · target 2000')
})

test("today's open day and a day left out are no dots of what was eaten (#207)", () => {
  const eaten = { kcal: 360, protein: 20, carbs: 40, fat: 10 }
  const option = tdeeOption(
    [
      day('2026-10-20', { status: 'excluded', intake: { ...eaten, kcal: 5000 } }),
      day('2026-10-21', { intake: { ...eaten, kcal: 2100 } }),
      day('2026-10-22', { status: 'open', intake: eaten }),
    ],
    words,
    palette,
  )
  expect(series(option).Eaten).toEqual([null, 2100, null])
  expect(tooltipOf(option, 2)).toBe('2026-10-22')
})

test('a night is long, middle or short, each a series of its colour, and averaged', () => {
  expect([7, 6.5, 5.9].map(nightOf)).toEqual(['long', 'middle', 'short'])
  const option = sleepOption(days, words, palette)
  const s = series(option)
  expect(s['7 h or more']).toEqual([7.5, null, null])
  expect(s['6 to 7 h']).toEqual([null, null, null])
  expect(s['Under 6 h']).toEqual([null, null, 5.5])
  expect(s['7-day average']).toEqual([7.5, 7.5, 6.5])
  const svg = svgOf(option)
  expect(svg).toContain(palette.blue)
  expect(svg).toContain(palette.bad)
  // In hours and minutes, as everywhere.
  expect(tooltipOf(option, 2)).toBe('2026-10-22 · 5 h 30 · 7-day average 6 h 30')
})

test('the 7 days are averaged over the days that have a value', () => {
  expect(average7([7, null, 5, 8, 8, 8, 8, 8, 2])).toEqual([7, 7, 6, 6.67, 7, 7.2, 7.33, 7.5, 6.71])
  expect(average7([null, null])).toEqual([null, null])
})

test('the weeks are averaged back from the last day, a week without a value a gap', () => {
  expect(weekly([1, 2, 3, 4, 5, 6, 7, 8, 9])).toEqual([1.5, 6])
  expect(weekly([null, null, null, null, null, null, null, 7])).toEqual([null, 7])
  expect(weekly([])).toEqual([])
})

test('steps count as calibration counts them, on one axis', () => {
  const stepping = [
    day('2026-10-20', { steps: 9000 }),
    day('2026-10-21', { steps: 0 }),
    day('2026-10-22', { status: 'excluded', steps: 30000 }),
    day('2026-10-23', { status: 'open', steps: 1000 }),
  ]
  expect(stepsOf(stepping)).toEqual([9000, null, null, null])
  const option = stepsOption(stepping, words, palette)
  expect(tooltipOf(option, 0)).toBe('2026-10-20 · 9000 steps')
})

test("the weights' axis ends on its steps, and holds the weights alone", () => {
  expect(reachOf([84.1, 92.9, 80])).toEqual({ min: 80, max: 95, interval: 5 })
  expect(reachOf([84.1, 86.3])).toEqual({ min: 84, max: 86.5, interval: 0.5 })
  expect(reachOf([80])).toEqual({ min: 79, max: 81, interval: 0.5 })
  expect(reachOf([])).toEqual({ scale: true })
  // A goal of 75 would stretch it to 74, the weights a strip at its top.
  const svg = svgOf(weightOption(days, words, palette, 75))
  expect(svg).not.toContain('>74.0<')
  expect(svg).toContain('>82.6<')
})

test('a measurement is one line on one axis, which writes it as told', () => {
  const option = lineOption(days, words, palette, {
    name: 'HRV',
    color: palette.primary,
    values: [50, null, 52],
    axis: (n) => `${String(n)} ms`,
    tip: (v) => `${String(v)} ms`,
  })
  expect(Array.isArray(option.yAxis)).toBe(false)
  expect(axisLabelOf(option, 'yAxis', 50)).toBe('50 ms')
  expect(tooltipOf(option, 2)).toBe('2026-10-22 · 52 ms')
  expect(series(option).HRV).toEqual([50, null, 52])
})

test('a goal is drawn within the axis the weights reach, not far below them', () => {
  expect(goalShown(days, 82.5)).toBe(true)
  expect(goalShown(days, 75)).toBe(false)
  expect(goalShown(days, undefined)).toBe(false)
  expect(goalShown([day('2026-10-20')], 80)).toBe(false)
  expect(svgOf(weightOption(days, words, palette, 82.5))).toContain('Goal 82.5')
  expect(svgOf(weightOption(days, words, palette, 75))).not.toContain('Goal')
  expect(svgOf(weightOption(days, words, palette))).not.toContain('Goal')
})

test("the space between the TDEE and the 7 days' intake is a band, either way", () => {
  const eating = [2000, 2600, null, 2300].map((kcal, i) =>
    day(`2026-10-2${String(i)}`, {
      ...(kcal === null ? {} : { intake: { kcal, protein: 150, carbs: 200, fat: 60 } }),
      tdee: { kcal: 2400, half_width: 50, trusted: true, before_phase: false, to: '2026-10-23' },
    }),
  )
  expect(eaten7(eating)).toEqual([2000, 2300, 2300, 2300])
  const s = series(tdeeOption(eating, words, palette))
  // From the lower of the two, as high as they are apart.
  expect(s.gapFoot).toEqual([2000, 2300, 2300, 2300])
  expect(s['Eaten to TDEE, 7-day average']).toEqual([400, 100, 100, 100])
  // None without a TDEE.
  const intake = { kcal: 2000, protein: 150, carbs: 200, fat: 60 }
  const before = series(tdeeOption([day('2026-10-20', { intake })], words, palette))
  expect(before.gapFoot).toEqual([null])
})

test('one weigh-in at the goal keeps the axis around it', () => {
  const option = weightOption(
    [day('2026-10-20', { weight_kg: 80, trend_kg: 80 })],
    words,
    palette,
    80,
  )
  const svg = svgOf(option)
  expect(svg).toContain('>81.0<')
  expect(svg).toContain('>79.0<')
  expect(svg).not.toContain('>40.0<')
})

test('one weigh-in without a goal keeps the axis around it too, and shows as a dot', () => {
  const option = weightOption([day('2026-10-20', { weight_kg: 80, trend_kg: 80 })], words, palette)
  const svg = svgOf(option)
  expect(svg).toContain('>81.0<')
  expect(svg).toContain('>79.0<')
  expect(JSON.stringify(option.series)).toContain('"showSymbol":true')
})

test('the axes write numbers and percentages as the language does', () => {
  const { result } = renderHook(() => useChartWords(false))
  const en = result.current
  expect(axisLabelOf(tdeeOption(days, en, palette), 'yAxis', 3034)).toBe('3,034')
  expect(axisLabelOf(weightOption(days, en, palette), 'yAxis', 85.5)).toBe('85.5')
  expect(axisLabelOf(weightOption(days, en, palette), 'xAxis', '2026-10-05')).toBe('Oct 5')
  expect(tooltipOf(weightOption(days, en, palette), 0)).toBe('Tue, Oct 20 · 82.4 kg · trend 82.6')
  expect(en.percent(32.7)).toBe('32.7%')

  act(() => {
    setLocale('hr')
  })
  const hr = result.current
  expect(axisLabelOf(tdeeOption(days, hr, palette), 'yAxis', 3034)).toBe('3.034')
  expect(axisLabelOf(weightOption(days, hr, palette), 'yAxis', 85.5)).toBe('85,5')
  expect(axisLabelOf(weightOption(days, hr, palette), 'xAxis', '2026-10-05')).toBe('5. lis')
  expect(tooltipOf(weightOption(days, hr, palette), 0)).toBe('uto 20. lis · 82,4 kg · trend 82,6')
  expect(hr.percent(32.7)).toBe('32,7 %')
})

test("a year's axis names each month once, with its year", () => {
  const { result } = renderHook(() => useChartWords(true))
  const option = weightOption(days, result.current, palette)
  expect(axisLabelOf(option, 'xAxis', '2026-10-01')).toBe('Oct 2026')
  expect(labelledOn(option, '2026-10-01')).toBe(true)
  expect(labelledOn(option, '2026-10-02')).toBe(false)
  // A shorter span's axis labels any day that fits.
  expect(labelledOn(weightOption(days, words, palette), '2026-10-02')).toBe(true)

  act(() => {
    setLocale('hr')
  })
  expect(axisLabelOf(weightOption(days, result.current, palette), 'xAxis', '2026-10-01')).toBe(
    'lis 2026.',
  )
})
