import { expect, test, vi } from 'vitest'

import {
  lineOption,
  sleepOption,
  stepsOption,
  tdeeOption,
  weightOption,
} from '@/components/charts/options'
import { palette } from '@/components/charts/palette'
import { day, svgOf, words } from '@/test/charts'

// This file is apart from options.test.ts: ECharts says what it no
// longer takes as it is once in a module's life, and the charts there
// are drawn before anything could listen.

const days = [
  day('2026-10-20', {
    meals: 1,
    intake: { kcal: 2000, protein: 150, carbs: 200, fat: 60 },
    target: { kcal: 2100, protein: 150, carbs: 220, fat: 60 },
    weight_kg: 82,
    trend_kg: 82,
    tdee: { kcal: 2400, half_width: 50, trusted: true, before_phase: false, to: '2026-10-20' },
    sleep_h: 7,
    steps: 9000,
    hrv: 50,
  }),
]

test('ECharts draws every chart without a word in development', () => {
  const log = vi.spyOn(console, 'log').mockImplementation(() => undefined)
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  for (const option of [
    weightOption(days, words, palette, 75),
    tdeeOption(days, words, palette),
    sleepOption(days, words, palette),
    stepsOption(days, words, palette),
    lineOption(days, words, palette, {
      name: 'HRV',
      color: palette.primary,
      values: [50],
      axis: (n) => String(n),
      tip: (v) => String(v),
    }),
  ]) {
    svgOf(option)
  }
  const said = [...log.mock.calls, ...warn.mock.calls, ...error.mock.calls]
    .map((args) => args.map(String).join(' '))
    .filter((s) => s.startsWith('[ECharts]'))
  log.mockRestore()
  warn.mockRestore()
  error.mockRestore()
  expect(said).toEqual([])
})
