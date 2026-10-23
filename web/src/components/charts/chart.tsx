import { BarChart, LineChart, ScatterChart } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
} from 'echarts/components'
import * as echarts from 'echarts/core'
import { SVGRenderer } from 'echarts/renderers'
import { useEffect, useRef } from 'react'

import type { Option } from './options'

// Only what the charts draw, so the bundle carries no more of ECharts.
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

// Chart draws an ECharts option, as wide as its card. It is for the eye:
// its card says in words what it shows.
export function Chart({ option }: { option: Option }) {
  const box = useRef<HTMLDivElement>(null)
  const chart = useRef<echarts.ECharts | null>(null)

  useEffect(() => {
    const el = box.current
    if (!el) {
      return undefined
    }
    const c = echarts.init(el, null, { renderer: 'svg', height: 220 })
    chart.current = c
    // A card that changes width (a phone turned) draws the chart again.
    const observer =
      typeof ResizeObserver === 'undefined'
        ? undefined
        : new ResizeObserver(() => {
            c.resize()
          })
    observer?.observe(el)
    return () => {
      observer?.disconnect()
      c.dispose()
      chart.current = null
    }
  }, [])

  useEffect(() => {
    chart.current?.setOption(option, true)
  }, [option])

  return <div ref={box} aria-hidden className="h-[220px] w-full" />
}
