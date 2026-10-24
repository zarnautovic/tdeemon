import { expect, test, vi } from 'vitest'

import {
  activityOption,
  bodyOption,
  macrosOption,
  recoveryOption,
  sleepOption,
  tdeeOption,
  weightOption,
} from '@/components/charts/options'
import { day, svgOf, words } from '@/test/charts'

// This file is apart from options.test.ts: ECharts says what it no
// longer takes as it is once in a module's life, and the charts there
// are drawn before anything could listen.

const days = [
  day('2026-10-20', {
    meals: 1,
    intake: { kcal: 2000, protein: 150, carbs: 200, fat: 60 },
    weight_kg: 82,
    trend_kg: 82,
    sleep_h: 7,
    steps: 9000,
    active_kcal: 400,
  }),
]

test('ECharts draws every chart without a word in development', () => {
  const log = vi.spyOn(console, 'log').mockImplementation(() => undefined)
  const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
  const error = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  for (const option of [
    weightOption(days, words, 75),
    macrosOption(days, words),
    tdeeOption(days, words),
    sleepOption(days, words),
    recoveryOption(days, words),
    bodyOption(days, words),
    activityOption(days, words),
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
