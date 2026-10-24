import { BarChart, LineChart, ScatterChart } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
} from 'echarts/components'
import * as echarts from 'echarts/core'
import { SVGRenderer } from 'echarts/renderers'

import type { HistoryDay, Words } from '@/components/charts/options'

// The parts of ECharts the app draws with, as Chart has them.
echarts.use([
  BarChart,
  LineChart,
  ScatterChart,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
  SVGRenderer,
])

// words are the charts' words in English, a date as its month and day.
export const words: Words = {
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

// day is a day of the history with nothing in it but over.
export function day(date: string, over: Partial<HistoryDay> = {}): HistoryDay {
  return { date, status: 'complete', type: 'rest', meals: 0, ...over }
}

// svgOf is an option drawn as the server would, without a browser.
export function svgOf(option: Record<string, unknown>): string {
  const chart = echarts.init(null, null, { renderer: 'svg', ssr: true, width: 360, height: 220 })
  chart.setOption(option)
  const svg = chart.renderToSVGString()
  chart.dispose()
  return svg
}
