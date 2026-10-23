import type { components } from '@/api/schema'

import { palette } from './palette'

export type HistoryDay = components['schemas']['HistoryDayBody']

// Words is what the options take of the app's language: the series'
// names, and how a date and a number are written.
export interface Words {
  date: (date: string) => string
  number: (n: number, digits?: number) => string
  names: {
    weighIn: string
    trend: string
    goal: string
    protein: string
    carbs: string
    fat: string
    target: string
    tdee: string
    interval: string
    intake: string
    sleep: string
    average: string
    rhr: string
    hrv: string
    weight: string
    bodyFat: string
    steps: string
    activeKcal: string
  }
}

// An option is ECharts', as the charts give it to setOption.
export type Option = Record<string, unknown>

// A value of a series: null where the day has none, a gap.
type Point = number | null

function value<T>(v: T | undefined): T | null {
  return v ?? null
}

// base is what every chart shares: the dates along the bottom, the grid's
// lines and the text in the palette's muted colour, a tooltip by day.
function base(days: HistoryDay[], w: Words, right = false): Option {
  return {
    backgroundColor: 'transparent',
    animation: false,
    textStyle: { color: palette.muted, fontFamily: 'inherit' },
    // The axes' labels fit in, however wide their numbers.
    grid: { left: 4, right: right ? 4 : 12, top: 32, bottom: 4, containLabel: true },
    legend: legend(),
    tooltip: {
      trigger: 'axis',
      backgroundColor: palette.surface,
      borderColor: palette.border,
      textStyle: { color: palette.text },
    },
    xAxis: {
      type: 'category',
      data: days.map((d) => w.date(d.date)),
      axisLine: { lineStyle: { color: palette.grid } },
      axisTick: { show: false },
      axisLabel: { color: palette.muted, hideOverlap: true },
    },
  }
}

// legend is the series' names over a chart, apart enough not to run
// into each other on a phone.
function legend(extra: Option = {}): Option {
  return {
    top: 0,
    itemGap: 16,
    itemHeight: 8,
    textStyle: { color: palette.muted, fontSize: 11 },
    ...extra,
  }
}

function axis(extra: Option = {}): Option {
  return {
    type: 'value',
    splitLine: { lineStyle: { color: palette.grid } },
    axisLabel: { color: palette.muted },
    ...extra,
  }
}

// weightOption is the weigh-ins as dots and the trend weight as a line,
// with the phase's goal as a dashed line when it has one.
export function weightOption(days: HistoryDay[], w: Words, goalKg?: number): Option {
  return {
    ...base(days, w),
    yAxis: axis({ scale: true }),
    series: [
      {
        name: w.names.weighIn,
        type: 'scatter',
        symbolSize: 6,
        itemStyle: { color: palette.dimBlue },
        data: days.map((d): Point => value(d.weight_kg)),
      },
      {
        name: w.names.trend,
        type: 'line',
        showSymbol: false,
        connectNulls: true,
        lineStyle: { color: palette.accent, width: 2.4 },
        itemStyle: { color: palette.accent },
        data: days.map((d): Point => value(d.trend_kg)),
        ...(goalKg === undefined
          ? {}
          : {
              markLine: {
                symbol: 'none',
                lineStyle: { color: palette.warn, type: 'dashed' },
                label: { color: palette.warn, formatter: `${w.names.goal} ${w.number(goalKg, 1)}` },
                data: [{ yAxis: goalKg }],
              },
            }),
      },
    ],
  }
}

// macrosOption is each day's protein, carbs and fat, in kcal, stacked, and
// the target of the day's type as a dashed line.
export function macrosOption(days: HistoryDay[], w: Words): Option {
  const bar = (name: string, color: string, kcal: (d: HistoryDay) => number | undefined) => ({
    name,
    type: 'bar',
    stack: 'kcal',
    barMaxWidth: 18,
    itemStyle: { color },
    data: days.map((d): Point => value(kcal(d))),
  })
  return {
    ...base(days, w),
    yAxis: axis(),
    series: [
      bar(w.names.protein, palette.blue, (d) => d.intake && Math.round(d.intake.protein * 4)),
      bar(w.names.carbs, palette.accent, (d) => d.intake && Math.round(d.intake.carbs * 4)),
      bar(w.names.fat, palette.warn, (d) => d.intake && Math.round(d.intake.fat * 9)),
      {
        name: w.names.target,
        type: 'line',
        step: 'middle',
        showSymbol: false,
        lineStyle: { color: palette.bad, type: 'dashed', width: 1.4 },
        itemStyle: { color: palette.bad },
        data: days.map((d): Point => value(d.target?.kcal)),
      },
    ],
  }
}

// tdeeOption is the TDEE measured up to each day, with its 80 % interval
// as a band, and what was eaten as dots.
export function tdeeOption(days: HistoryDay[], w: Words): Option {
  return {
    ...base(days, w),
    yAxis: axis({ scale: true }),
    series: [
      {
        // The band's foot, not drawn: the band stacks on it.
        name: 'low',
        type: 'line',
        stack: 'interval',
        showSymbol: false,
        lineStyle: { opacity: 0 },
        tooltip: { show: false },
        data: days.map((d): Point => (d.tdee ? d.tdee.kcal - d.tdee.half_width : null)),
      },
      {
        name: w.names.interval,
        type: 'line',
        stack: 'interval',
        showSymbol: false,
        lineStyle: { opacity: 0 },
        areaStyle: { color: palette.series2, opacity: 0.18 },
        data: days.map((d): Point => (d.tdee ? 2 * d.tdee.half_width : null)),
      },
      {
        name: w.names.tdee,
        type: 'line',
        showSymbol: false,
        lineStyle: { color: palette.series2, width: 2.4 },
        itemStyle: { color: palette.series2 },
        data: days.map((d): Point => value(d.tdee?.kcal)),
      },
      {
        name: w.names.intake,
        type: 'scatter',
        symbolSize: 5,
        itemStyle: { color: palette.muted },
        data: days.map((d): Point => value(d.intake?.kcal)),
      },
    ],
    legend: legend({ data: [w.names.tdee, w.names.interval, w.names.intake] }),
  }
}

// sleepColor is a night's colour: 7 hours and more blue, 6 amber, less red.
export function sleepColor(hours: number): string {
  if (hours >= 7) {
    return palette.blue
  }
  return hours >= 6 ? palette.warn : palette.bad
}

// average7 is the average of each day's 7 days up to it that have a value.
export function average7(values: Point[]): Point[] {
  return values.map((_, i) => {
    const week = values.slice(Math.max(0, i - 6), i + 1).filter((v): v is number => v !== null)
    return week.length === 0
      ? null
      : Math.round((week.reduce((a, b) => a + b, 0) / week.length) * 100) / 100
  })
}

// sleepOption is each night's sleep as a bar coloured by its length, and
// the 7 days' average as a dashed line.
export function sleepOption(days: HistoryDay[], w: Words): Option {
  const hours = days.map((d): Point => value(d.sleep_h))
  return {
    ...base(days, w),
    yAxis: axis({ min: 0 }),
    series: [
      {
        name: w.names.sleep,
        type: 'bar',
        barMaxWidth: 18,
        itemStyle: { color: palette.blue },
        data: hours.map((h) =>
          h === null ? null : { value: h, itemStyle: { color: sleepColor(h) } },
        ),
      },
      {
        name: w.names.average,
        type: 'line',
        showSymbol: false,
        connectNulls: true,
        lineStyle: { color: palette.muted, type: 'dashed', width: 1.6 },
        itemStyle: { color: palette.muted },
        data: average7(hours),
      },
    ],
  }
}

// twoAxes is a chart of two lines, each on an axis of its own.
function twoAxes(
  days: HistoryDay[],
  w: Words,
  left: { name: string; color: string; data: Point[]; area?: boolean },
  right: { name: string; color: string; data: Point[] },
): Option {
  return {
    ...base(days, w, true),
    yAxis: [
      axis({ scale: true }),
      axis({ scale: true, splitLine: { show: false }, axisLabel: { color: right.color } }),
    ],
    series: [
      {
        name: left.name,
        type: 'line',
        showSymbol: false,
        connectNulls: true,
        lineStyle: { color: left.color, width: 2.4 },
        itemStyle: { color: left.color },
        ...(left.area ? { areaStyle: { color: left.color, opacity: 0.07 } } : {}),
        data: left.data,
      },
      {
        name: right.name,
        type: 'line',
        yAxisIndex: 1,
        showSymbol: false,
        connectNulls: true,
        lineStyle: { color: right.color, width: 2 },
        itemStyle: { color: right.color },
        data: right.data,
      },
    ],
  }
}

// recoveryOption is the resting heart rate and the HRV, each on its axis.
export function recoveryOption(days: HistoryDay[], w: Words): Option {
  return twoAxes(
    days,
    w,
    { name: w.names.rhr, color: palette.bad, data: days.map((d): Point => value(d.resting_hr)) },
    { name: w.names.hrv, color: palette.accent, data: days.map((d): Point => value(d.hrv)) },
  )
}

// bodyOption is the trend weight and the body fat, each on its axis.
export function bodyOption(days: HistoryDay[], w: Words): Option {
  return twoAxes(
    days,
    w,
    {
      name: w.names.weight,
      color: palette.accent,
      data: days.map((d): Point => value(d.trend_kg)),
      area: true,
    },
    {
      name: w.names.bodyFat,
      color: palette.warn,
      data: days.map((d): Point => value(d.body_fat_pct)),
    },
  )
}

// activityOption is each day's steps as bars and its active energy as a
// line on an axis of its own.
export function activityOption(days: HistoryDay[], w: Words): Option {
  return {
    ...base(days, w, true),
    yAxis: [
      axis({ min: 0 }),
      axis({ min: 0, splitLine: { show: false }, axisLabel: { color: palette.accent } }),
    ],
    series: [
      {
        name: w.names.steps,
        type: 'bar',
        barMaxWidth: 18,
        itemStyle: { color: palette.steps },
        data: days.map((d): Point => value(d.steps)),
      },
      {
        name: w.names.activeKcal,
        type: 'line',
        yAxisIndex: 1,
        symbolSize: 4,
        lineStyle: { color: palette.accent, width: 2 },
        itemStyle: { color: palette.accent },
        data: days.map((d): Point => value(d.active_kcal)),
      },
    ],
  }
}
