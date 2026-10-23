import { keepPreviousData } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'

import { $api } from '@/api/client'
import { Chart } from '@/components/charts/chart'
import {
  activityOption,
  bodyOption,
  macrosOption,
  recoveryOption,
  sleepOption,
  tdeeOption,
  weightOption,
  type HistoryDay,
  type Option,
  type Words,
} from '@/components/charts/options'
import { palette } from '@/components/charts/palette'
import { LoadError } from '@/components/load-error'
import { Button } from '@/components/ui/button'
import { addDays, useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// The spans the charts show, in days up to today.
const spans = [30, 90, 180, 365] as const

// averageOf is the average of the days' values that have one.
function averageOf(values: (number | undefined)[]): number | undefined {
  const have = values.filter((v): v is number => v !== undefined)
  return have.length === 0 ? undefined : have.reduce((a, b) => a + b, 0) / have.length
}

// ChartsPage is the user's days as charts, over a span up to today: the
// weight and its trend, what was eaten against the target, the TDEE with
// its interval, sleep and recovery, the body and activity. Each says in
// words what it shows too.
export function ChartsPage() {
  const t = useMessages()
  const f = useFormat()
  const { days: span = 90 } = useSearch({ from: '/app/charts' })
  const navigate = useNavigate({ from: '/charts' })
  const me = $api.useQuery('get', '/api/v1/me')
  const today = me.data?.today
  const history = $api.useQuery(
    'get',
    '/api/v1/history',
    { params: { query: { from: today === undefined ? '' : addDays(today, 1 - span) } } },
    { enabled: today !== undefined, placeholderData: keepPreviousData },
  )

  const words: Words = {
    date: f.short,
    number: f.number,
    names: t.charts.names,
  }

  let body
  if (me.isError) {
    body = <LoadError error={me.error} onRetry={() => void me.refetch()} />
  } else if (history.isError) {
    body = <LoadError error={history.error} onRetry={() => void history.refetch()} />
  } else if (!history.data) {
    body = <output className="block text-muted-foreground">{t.loading}</output>
  } else {
    body = (
      <Cards
        days={history.data.days}
        words={words}
        goalKg={me.data?.phase?.goal_weight}
        busy={history.isPlaceholderData}
      />
    )
  }

  return (
    <section className="space-y-4">
      <h1 className="text-2xl font-semibold">{t.charts.title}</h1>
      <fieldset aria-label={t.charts.span} className="flex gap-1">
        {spans.map((n) => (
          <Button
            key={n}
            variant={span === n ? 'default' : 'outline'}
            aria-pressed={span === n}
            onClick={() => {
              void navigate({ search: { days: n === 90 ? undefined : n }, replace: true })
            }}
          >
            {t.charts.days(n)}
          </Button>
        ))}
      </fieldset>
      {body}
    </section>
  )
}

// Cards are the charts of days.
function Cards({
  days,
  words,
  goalKg,
  busy,
}: {
  days: HistoryDay[]
  words: Words
  goalKg: number | undefined
  busy: boolean
}) {
  const t = useMessages()
  const f = useFormat()
  const n = (v: number | undefined, digits = 0) =>
    v === undefined ? undefined : f.number(v, digits)
  const logged = days.filter((d) => d.intake !== undefined)
  if (
    logged.length === 0 &&
    days.every((d) => d.weight_kg === undefined && d.steps === undefined && d.sleep_h === undefined)
  ) {
    return <p className="text-muted-foreground">{t.charts.none}</p>
  }
  const trends = days.flatMap((d) => (d.trend_kg === undefined ? [] : [d.trend_kg]))
  const lastTDEE = days.findLast((d) => d.tdee !== undefined)?.tdee
  const lastFat = days.findLast((d) => d.body_fat_pct !== undefined)?.body_fat_pct
  const kcal = n(averageOf(logged.map((d) => d.intake?.kcal)))
  const sleep = n(averageOf(days.map((d) => d.sleep_h)), 1)
  const rhr = n(averageOf(days.map((d) => d.resting_hr)))
  const hrv = n(averageOf(days.map((d) => d.hrv)))
  const steps = n(averageOf(days.map((d) => d.steps)))
  const none = t.charts.said.none

  const cards: { title: string; said: string; option: Option }[] = [
    {
      title: t.charts.weight,
      said:
        trends.length > 0
          ? t.charts.said.weight(f.number(trends[0] ?? 0, 1), f.number(trends.at(-1) ?? 0, 1))
          : none,
      option: weightOption(days, words, goalKg),
    },
    {
      title: t.charts.macros,
      said: kcal === undefined ? none : t.charts.said.macros(kcal, logged.length),
      option: macrosOption(days, words),
    },
    {
      title: t.charts.tdee,
      said: lastTDEE
        ? t.charts.said.tdee(f.number(lastTDEE.kcal), f.number(lastTDEE.half_width))
        : none,
      option: tdeeOption(days, words),
    },
    {
      title: t.charts.sleep,
      said: sleep === undefined ? none : t.charts.said.sleep(sleep),
      option: sleepOption(days, words),
    },
    {
      title: t.charts.recovery,
      said: rhr === undefined || hrv === undefined ? none : t.charts.said.recovery(rhr, hrv),
      option: recoveryOption(days, words),
    },
    {
      title: t.charts.body,
      said: lastFat === undefined ? none : t.charts.said.body(f.number(lastFat, 1)),
      option: bodyOption(days, words),
    },
    {
      title: t.charts.activity,
      said: steps === undefined ? none : t.charts.said.activity(steps),
      option: activityOption(days, words),
    },
  ]

  return (
    <div aria-busy={busy} className={`grid gap-4 md:grid-cols-2 ${busy ? 'opacity-60' : ''}`}>
      {cards.map((c) => (
        <section
          key={c.title}
          aria-label={c.title}
          className="space-y-2 rounded-[14px] border p-4"
          style={{ background: palette.surface, borderColor: palette.border, color: palette.text }}
        >
          <h2 className="text-xs tracking-widest uppercase" style={{ color: palette.muted }}>
            {c.title}
          </h2>
          <p className="text-sm">{c.said}</p>
          <Chart option={c.option} />
        </section>
      ))}
    </div>
  )
}
