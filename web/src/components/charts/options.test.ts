import { LineChart, ScatterChart } from 'echarts/charts'
import { GridComponent, LegendComponent, MarkLineComponent } from 'echarts/components'
import * as echarts from 'echarts/core'
import { SVGRenderer } from 'echarts/renderers'
import { expect, test } from 'vitest'

import {
  average7,
  macrosOption,
  sleepColor,
  tdeeOption,
  weightOption,
  type HistoryDay,
  type Words,
} from '@/components/charts/options'
import { palette } from '@/components/charts/palette'

const words: Words = {
  date: (d) => d.slice(5),
  number: (n, digits = 0) => n.toFixed(digits),
  names: {
    weighIn: 'Weigh-in',
    trend: 'Trend',
    goal: 'Goal',
    protein: 'Protein',
    carbs: 'Carbs',
    fat: 'Fat',
    target: 'Target',
    tdee: 'TDEE',
    interval: 'Interval',
    intake: 'Eaten',
    sleep: 'Sleep',
    average: '7 days',
    rhr: 'RHR',
    hrv: 'HRV',
    weight: 'Weight',
    bodyFat: 'Body fat',
    steps: 'Steps',
    activeKcal: 'Active',
  },
}

function day(date: string, over: Partial<HistoryDay> = {}): HistoryDay {
  return { date, status: 'complete', type: 'rest', meals: 0, ...over }
}

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

// svgOf is an option drawn as the server would, without a browser.
function svgOf(option: Record<string, unknown>): string {
  echarts.use([
    LineChart,
    ScatterChart,
    GridComponent,
    LegendComponent,
    MarkLineComponent,
    SVGRenderer,
  ])
  const chart = echarts.init(null, null, { renderer: 'svg', ssr: true, width: 360, height: 220 })
  chart.setOption(option)
  const svg = chart.renderToSVGString()
  chart.dispose()
  return svg
}

test('the goal is drawn though it is far below the weights', () => {
  const svg = svgOf(weightOption(days, words, 75))

  expect(svg).toContain('Goal 75.0')
  expect(svgOf(weightOption(days, words))).not.toContain('Goal')
})
