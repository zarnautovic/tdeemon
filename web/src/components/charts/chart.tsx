import { BarChart, LineChart, ScatterChart } from 'echarts/charts'
import { GridComponent, MarkLineComponent, TooltipComponent } from 'echarts/components'
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
  MarkLineComponent,
  TooltipComponent,
  SVGRenderer,
])

function textStyleOf(option: Option): Record<string, unknown> {
  const style: unknown = option.textStyle
  return typeof style === 'object' && style !== null ? { ...style } : {}
}

// Chart draws an ECharts option, as wide as its card and height pixels
// high (the height it starts with, for its life). To a screen reader it
// is an image, which label sums up; the page has its values as text too
// (the weeks' table, the cards' numbers). An option of new colours (the
// theme changed) draws it again.
export function Chart({
  option,
  label,
  height = 240,
}: {
  option: Option
  label: string
  height?: number
}) {
  const box = useRef<HTMLDivElement>(null)
  const chart = useRef<echarts.ECharts | null>(null)
  const tall = useRef(height)

  useEffect(() => {
    const el = box.current
    if (!el) {
      return undefined
    }
    const c = echarts.init(el, null, { renderer: 'svg', height: tall.current })
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
    // ECharts measures text by the font's name, which "inherit" is not.
    const font = getComputedStyle(document.body).fontFamily || 'sans-serif'
    chart.current?.setOption(
      { ...option, textStyle: { ...textStyleOf(option), fontFamily: font } },
      true,
    )
  }, [option])

  return (
    <div
      ref={box}
      // oxlint-disable-next-line jsx-a11y/prefer-tag-over-role -- ECharts draws into a box, which an <img> cannot be
      role="img"
      aria-label={label}
      className="w-full"
      style={{ height: `${String(height)}px` }}
    />
  )
}
