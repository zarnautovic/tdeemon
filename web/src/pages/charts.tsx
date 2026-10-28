import { keepPreviousData } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { ChartLine } from 'lucide-react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { Legend } from '@/components/charts/legend'
import {
  counted,
  lineOption,
  nightOf,
  sleepOption,
  stepsOf,
  stepsOption,
  tdeeOption,
  weightOption,
  type HistoryDay,
  type Night,
  type Point,
  type Words,
} from '@/components/charts/options'
import { useChartColors, type ChartColors } from '@/components/charts/palette'
import { useChartWords } from '@/components/charts/words'
import { LoadError } from '@/components/load-error'
import { ChartCard } from '@/components/progress/chart-card'
import { MetricTile } from '@/components/progress/metric-tile'
import { Skeleton } from '@/components/progress/skeleton'
import { byMonth, firstDay, spans, weeksOf, type Span } from '@/components/progress/span'
import { card, entry } from '@/components/progress/styles'
import { VerdictHero } from '@/components/progress/verdict-hero'
import { WeeklyTable } from '@/components/progress/weekly-table'
import { buttonVariants } from '@/components/ui/button'
import { SegmentedControl } from '@/components/ui/segmented'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

type Phase = components['schemas']['PhaseBody']

type Days = 30 | 180 | 365 | 'phase' | undefined

// The address's days of each span: 90 is the default, left out.
const searchOf: Record<Span, Days> = {
  '30': 30,
  '90': undefined,
  '180': 180,
  '365': 365,
  phase: 'phase',
}

// spanOf is the span the address's days say.
function spanOf(days: Days): Span {
  return spans.find((s) => searchOf[s] === days) ?? '90'
}

// drawn reports whether a day has anything a chart draws.
function drawn(d: HistoryDay): boolean {
  return [
    d.intake,
    d.weight_kg,
    d.trend_kg,
    d.tdee,
    d.sleep_h,
    d.resting_hr,
    d.hrv,
    d.body_fat_pct,
    d.steps,
  ].some((v) => v !== undefined)
}

// averageOf is the average of the values there are.
function averageOf(values: Point[]): number | undefined {
  const have = values.filter((v): v is number => v !== null)
  return have.length === 0 ? undefined : have.reduce((a, b) => a + b, 0) / have.length
}

// ChartsPage is the progress over a period up to today (30 days to a year,
// or the phase's): first how the phase goes, as calibration judges it;
// then the weight and its trend, and the TDEE against what was eaten and
// the target; the weeks in a table, which says the charts in text; and
// sleep, the heart, the steps and the body fat in small, each a whole
// chart a tap away.
export function ChartsPage() {
  const t = useMessages()
  const { days: asked } = useSearch({ from: '/app/progress' })
  const navigate = useNavigate({ from: '/progress' })
  const me = $api.useQuery('get', '/api/v1/me')
  const today = me.data?.today
  const phase = me.data?.phase
  // The phase's span is there while the user is in one; without, the
  // default's.
  const noPhase = me.data !== undefined && phase === undefined
  const span = asked === 'phase' && noPhase ? '90' : spanOf(asked)
  const from = today === undefined ? undefined : firstDay(span, today, phase?.start)
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query: { from: from ?? '' } } },
    { enabled: from !== undefined, placeholderData: keepPreviousData },
  )
  const words = useChartWords(from !== undefined && today !== undefined && byMonth(from, today))
  const colors = useChartColors()

  let body
  if (me.isError) {
    body = <LoadError error={me.error} onRetry={() => void me.refetch()} />
  } else if (history.isError) {
    body = <LoadError error={history.error} onRetry={() => void history.refetch()} />
  } else if (!history.data || today === undefined || from === undefined) {
    body = <ProgressSkeleton />
  } else if (history.data.days.every((d) => !drawn(d))) {
    body = <Empty />
  } else {
    body = (
      <Progress
        days={history.data.days}
        words={words}
        colors={colors}
        phase={phase}
        weeks={weeksOf(from, today)}
        today={today}
        busy={history.isPlaceholderData}
      />
    )
  }

  return (
    <section className="space-y-3 lg:space-y-4">
      <header className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <h1 className="text-title">{t.charts.title}</h1>
        <SegmentedControl
          legend={t.charts.span}
          hideLegend
          className="lg:w-104"
          options={spans
            .filter((s) => s !== 'phase' || !noPhase)
            .map((s) => ({
              value: s,
              label: (
                <>
                  <span aria-hidden>{t.charts.spans[s]}</span>
                  <span className="sr-only">{t.charts.spansSaid[s]}</span>
                </>
              ),
            }))}
          value={span}
          onChange={(s) => {
            void navigate({ search: { days: searchOf[s] }, replace: true })
          }}
        />
      </header>
      {today === undefined ? (
        !me.isError && <HeroSkeleton />
      ) : (
        <VerdictHero phase={phase} today={today} />
      )}
      {body}
    </section>
  )
}

// Progress is the charts of the days, the table of the weeks and the
// measurements in small.
function Progress({
  days,
  words,
  colors,
  phase,
  weeks,
  today,
  busy,
}: {
  days: HistoryDay[]
  words: Words
  colors: ChartColors
  phase: Phase | undefined
  weeks: { from: string; to: string; count: number }
  today: string
  busy: boolean
}) {
  const t = useMessages()
  const f = useFormat()
  const said = t.charts.said
  const none = said.none

  const trends = days.flatMap((d) => (d.trend_kg === undefined ? [] : [d.trend_kg]))
  const firstTrend = trends[0]
  const lastTrend = trends.at(-1)
  const weighIns = days.filter((d) => d.weight_kg !== undefined).length
  const lastTDEE = days.findLast((d) => d.tdee !== undefined)?.tdee
  // The days logged, as calibration counts them: over, with a meal, not
  // left out. Today is open, its intake not all eaten yet.
  const logged = days.filter((d) => counted(d) && d.intake !== undefined)
  const kcal = averageOf(logged.map((d) => d.intake?.kcal ?? null))

  const weightLabel =
    firstTrend === undefined || lastTrend === undefined
      ? none
      : said.weight(f.number(firstTrend, 1), f.number(lastTrend, 1), weighIns)
  const tdeeSaid = lastTDEE
    ? said.tdee(
        f.number(lastTDEE.kcal),
        f.number(lastTDEE.half_width),
        lastTDEE.trusted ? (lastTDEE.before_phase ? 'before' : 'trusted') : 'unsure',
      )
    : undefined
  const eatenSaid = kcal === undefined ? undefined : said.eaten(f.number(kcal), logged.length)
  const names = words.names
  const goal = phase?.goal_weight

  return (
    <div
      aria-busy={busy}
      className={cn('space-y-3 transition-opacity lg:space-y-4', busy && 'opacity-60')}
    >
      <div className="grid gap-3 lg:grid-cols-2 lg:gap-4">
        <ChartCard
          title={t.charts.weight}
          value={lastTrend === undefined ? undefined : t.charts.tips.kg(f.number(lastTrend, 1))}
          caption={t.charts.trend}
          note={
            firstTrend === undefined || lastTrend === undefined || trends.length < 2
              ? undefined
              : t.charts.change(f.signed(lastTrend - firstTrend, 1))
          }
          marks={[
            { name: names.weighIn, mark: 'dot', color: 'bg-chart-5' },
            { name: names.trend, mark: 'line', color: 'bg-chart-1' },
            ...(goal === undefined
              ? []
              : [{ name: names.goal, mark: 'dashed' as const, color: 'border-warning' }]),
          ]}
          option={weightOption(days, words, colors, goal)}
          label={weightLabel}
        />
        <ChartCard
          title={t.charts.tdee}
          value={lastTDEE ? t.unit.kcal(f.number(lastTDEE.kcal)) : undefined}
          caption={lastTDEE ? t.charts.tdeeNow(f.number(lastTDEE.half_width)) : undefined}
          note={kcal === undefined ? undefined : t.charts.eaten(f.number(kcal), logged.length)}
          marks={[
            { name: names.tdee, mark: 'line', color: 'bg-chart-2' },
            { name: names.interval, mark: 'band', color: 'bg-chart-2' },
            { name: names.intake, mark: 'dot', color: 'bg-chart-5' },
            { name: names.target, mark: 'dashed', color: 'border-warning' },
          ]}
          option={tdeeOption(days, words, colors)}
          label={[tdeeSaid, eatenSaid].filter((s) => s !== undefined).join(' ') || none}
        />
      </div>
      {/* A desktop's weeks and measurements side by side, a phone's one
          after the other. */}
      <div className="grid gap-3 lg:grid-cols-2 lg:items-start lg:gap-4">
        {weeks.count > 0 && <WeeklyTable weeks={weeks} today={today} />}
        <Measurements days={days} words={words} colors={colors} />
      </div>
    </div>
  )
}

// The nights' classes by their length, as the charts' colours.
const nights: Record<Night, string> = {
  long: 'fill-chart-2',
  middle: 'fill-chart-3',
  short: 'fill-destructive',
}

// Measurements are sleep, the resting heart rate, the HRV, the steps and
// the body fat in small, each that has a value in the days.
function Measurements({
  days,
  words,
  colors,
}: {
  days: HistoryDay[]
  words: Words
  colors: ChartColors
}) {
  const t = useMessages()
  const f = useFormat()
  const said = t.charts.said
  const tips = t.charts.tips

  const sleep = days.map((d): Point => d.sleep_h ?? null)
  const rhr = days.map((d): Point => d.resting_hr ?? null)
  const hrv = days.map((d): Point => d.hrv ?? null)
  const steps = stepsOf(days)
  const fat = days.map((d): Point => d.body_fat_pct ?? null)
  const sleepAverage = averageOf(sleep)
  const rhrAverage = averageOf(rhr)
  const hrvAverage = averageOf(hrv)
  const stepsAverage = averageOf(steps)
  const lastFat = fat.findLast((v): v is number => v !== null)

  const tiles = [
    sleepAverage !== undefined && (
      <MetricTile
        key="sleep"
        title={t.charts.sleep}
        value={tips.hours(f.number(sleepAverage, 1))}
        caption={t.charts.average}
        said={said.sleep(f.number(sleepAverage, 1))}
        values={sleep}
        bars
        classOf={(h) => nights[nightOf(h)]}
        legend={<SleepLegend />}
        chartLegend={<SleepLegend average />}
        option={sleepOption(days, words, colors)}
      />
    ),
    rhrAverage !== undefined && (
      <MetricTile
        key="rhr"
        title={t.charts.rhr}
        value={tips.bpm(f.number(rhrAverage))}
        caption={t.charts.average}
        said={said.rhr(f.number(rhrAverage))}
        values={rhr}
        className="stroke-chart-4"
        option={lineOption(days, words, colors, {
          name: words.names.rhr,
          color: colors.pink,
          values: rhr,
          axis: (n) => words.number(n),
          tip: (v) => tips.bpm(f.number(v)),
        })}
      />
    ),
    hrvAverage !== undefined && (
      <MetricTile
        key="hrv"
        title={t.charts.hrv}
        value={tips.ms(f.number(hrvAverage))}
        caption={t.charts.average}
        said={said.hrv(f.number(hrvAverage))}
        values={hrv}
        className="stroke-chart-1"
        option={lineOption(days, words, colors, {
          name: words.names.hrv,
          color: colors.primary,
          values: hrv,
          axis: (n) => words.number(n),
          tip: (v) => tips.ms(f.number(v)),
        })}
      />
    ),
    stepsAverage !== undefined && (
      <MetricTile
        key="steps"
        title={t.charts.steps}
        value={f.number(stepsAverage)}
        caption={t.charts.average}
        said={said.steps(f.number(stepsAverage))}
        values={steps}
        bars
        className="fill-chart-2"
        option={stepsOption(days, words, colors)}
      />
    ),
    lastFat !== undefined && (
      <MetricTile
        key="fat"
        title={t.charts.bodyFat}
        value={f.percent(lastFat, 1)}
        caption={t.charts.latest}
        said={said.bodyFat(f.percent(lastFat, 1))}
        values={fat}
        className="stroke-chart-3"
        option={lineOption(days, words, colors, {
          name: words.names.bodyFat,
          color: colors.amber,
          values: fat,
          axis: (n) => words.percent(n),
          tip: (v) => f.percent(v, 1),
        })}
      />
    ),
  ].filter((tile) => tile !== false)

  if (tiles.length === 0) {
    return null
  }
  return (
    <section aria-labelledby="progress-body" className="space-y-2">
      <h2 id="progress-body" className="text-h3">
        {t.charts.body}
      </h2>
      <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-2 lg:gap-4 xl:grid-cols-3">
        {tiles}
      </ul>
    </section>
  )
}

// SleepLegend says what the colours of the nights' bars mean, and of the
// whole chart, its 7 days' average (average).
function SleepLegend({ average = false }: { average?: boolean }) {
  const names = useMessages().charts.names

  return (
    <Legend
      marks={[
        { name: names.long, mark: 'bar', color: 'bg-chart-2' },
        { name: names.middle, mark: 'bar', color: 'bg-chart-3' },
        { name: names.short, mark: 'bar', color: 'bg-destructive' },
        ...(average
          ? [{ name: names.average, mark: 'dashed' as const, color: 'border-chart-5' }]
          : []),
      ]}
    />
  )
}

// Empty is the page of days with nothing logged, and the first thing to do.
function Empty() {
  const t = useMessages()

  return (
    <div className={cn(card, 'flex flex-col items-center gap-3 py-10 text-center', entry)}>
      <ChartLine aria-hidden className="size-8 text-muted-foreground" />
      <p className="text-h3">{t.charts.none}</p>
      <p className="max-w-sm text-small text-muted-foreground">{t.charts.noneHint}</p>
      <Link to="/" search={{ log: true }} className={buttonVariants()}>
        {t.charts.logMeal}
      </Link>
    </div>
  )
}

// HeroSkeleton stands in the hero's place until the app knows the day.
function HeroSkeleton() {
  return (
    <div className={cn(card, 'space-y-3 lg:p-6')}>
      <Skeleton className="h-4 w-40" />
      <Skeleton className="h-8 w-48" />
      <Skeleton className="h-5 w-full max-w-md" />
    </div>
  )
}

// ProgressSkeleton stands in the charts' place, in their shape, while
// their days come.
function ProgressSkeleton() {
  const t = useMessages()

  return (
    <div aria-busy className="space-y-3 lg:space-y-4">
      <output className="sr-only">{t.loading}</output>
      <div className="grid gap-3 lg:grid-cols-2 lg:gap-4">
        {[0, 1].map((i) => (
          <div key={i} className={cn(card, 'space-y-3')}>
            <div className="space-y-1.5">
              <Skeleton className="h-5 w-32" />
              <Skeleton className="h-4 w-48" />
            </div>
            <Skeleton className="h-4 w-56" />
            <Skeleton className="h-60" />
          </div>
        ))}
      </div>
      <div className="grid gap-3 lg:grid-cols-2 lg:items-start lg:gap-4">
        <div className={cn(card, 'space-y-3')}>
          <Skeleton className="h-5 w-24" />
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-7" />
          ))}
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-2 lg:gap-4 xl:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-36 rounded-lg" />
          ))}
        </div>
      </div>
    </div>
  )
}
