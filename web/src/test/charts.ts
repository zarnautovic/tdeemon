/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { BarChart, LineChart, ScatterChart } from 'echarts/charts'
import { GridComponent, MarkLineComponent, TooltipComponent } from 'echarts/components'
import * as echarts from 'echarts/core'
import { SVGRenderer } from 'echarts/renderers'

import type { HistoryDay, Words } from '@/components/charts/options'
import { en } from '@/i18n/en'

// The app's stylesheet, as it is written: Vitest gives a test none of a
// stylesheet's import, not even its text.
const css = readFileSync(join(import.meta.dirname, '..', 'index.css'), 'utf8')

// The parts of ECharts the app draws with, as Chart has them.
echarts.use([
  BarChart,
  LineChart,
  ScatterChart,
  GridComponent,
  MarkLineComponent,
  TooltipComponent,
  SVGRenderer,
])

// words are the charts' words in English, a date on the axis as its
// month and day, in a tooltip as itself, and a number with a full stop
// and no groups, as toFixed writes it.
export const words: Words = {
  axisDate: (date) => date.slice(5),
  months: false,
  day: (date) => date,
  number: (n, digits = 0) => n.toFixed(digits),
  percent: (n, digits = 1) => `${n.toFixed(digits)} %`,
  hm: (hours) => {
    const minutes = Math.round(hours * 60)
    return en.unit.hm(String(Math.floor(minutes / 60)), String(minutes % 60).padStart(2, '0'))
  },
  names: en.charts.names,
  tips: en.charts.tips,
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

// declarations are a rule's custom properties, by name.
function declarations(rule: string): Record<string, string> {
  return Object.fromEntries(
    [...rule.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map(([, name, value]) => [
      name ?? '',
      (value ?? '').trim(),
    ]),
  )
}

// themeTokens are index.css's tokens in each theme: :root's, and the dark
// theme's laid over them.
export function themeTokens(): { light: Record<string, string>; dark: Record<string, string> } {
  const root = css.slice(css.indexOf(':root {'))
  const variant = root.indexOf('@variant dark {')
  const light = declarations(root.slice(0, variant))
  const dark = { ...light, ...declarations(root.slice(variant, root.indexOf('}', variant))) }
  return { light, dark }
}

// block is tokens as a rule's declarations.
function block(tokens: Record<string, string>): string {
  return Object.entries(tokens)
    .map(([name, value]) => `${name}: ${value};`)
    .join(' ')
}

// themeStyle is a stylesheet of index.css's tokens alone, light, and dark
// under .dark: a test's page has no Tailwind to build the whole of it.
export function themeStyle(): HTMLStyleElement {
  const { light, dark } = themeTokens()
  const style = document.createElement('style')
  style.textContent = `:root { ${block(light)} } :root.dark { ${block(dark)} }`
  return style
}

// isFormatter tells a function of ECharts' params, as the options' are.
function isFormatter(f: unknown): f is (...params: unknown[]) => unknown {
  return typeof f === 'function'
}

// labelOf is an option's days' axis's label.
function labelOf(option: Record<string, unknown>): Record<string, unknown> {
  const axis: unknown = option.xAxis
  const label: unknown =
    typeof axis === 'object' && axis !== null && 'axisLabel' in axis ? axis.axisLabel : undefined
  return typeof label === 'object' && label !== null ? { ...label } : {}
}

// labelledOn reports whether an option's days' axis may label a date:
// any, unless its interval says which.
export function labelledOn(option: Record<string, unknown>, date: string): boolean {
  const { interval } = labelOf(option)
  return isFormatter(interval) ? interval(0, date) === true : true
}

// tooltipOf is what an option's tooltip says of the i-th day, as ECharts
// asks it.
export function tooltipOf(option: Record<string, unknown>, i: number): string {
  const tooltip: unknown = option.tooltip
  if (typeof tooltip !== 'object' || tooltip === null || !('formatter' in tooltip)) {
    return ''
  }
  const { formatter } = tooltip
  const said = isFormatter(formatter) ? formatter([{ dataIndex: i }]) : undefined
  return typeof said === 'string' ? said : ''
}

// axisLabelOf is what an option's axis writes of a value: the days' axis
// (x) of a date, the values' (y) of a number.
export function axisLabelOf(
  option: Record<string, unknown>,
  axis: 'xAxis' | 'yAxis',
  value: unknown,
): string {
  const of: unknown = option[axis]
  const label: unknown =
    typeof of === 'object' && of !== null && 'axisLabel' in of ? of.axisLabel : undefined
  const formatter: unknown =
    typeof label === 'object' && label !== null && 'formatter' in label
      ? label.formatter
      : undefined
  const said = isFormatter(formatter) ? formatter(value) : undefined
  return typeof said === 'string' ? said : ''
}
