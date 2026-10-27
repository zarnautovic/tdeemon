import { useState } from 'react'

import { $api } from '@/api/client'
import type { Metric } from '@/api/types'
import { DayMetrics } from '@/components/day/day-metrics'
import { Sheet } from '@/components/sheet'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// BodyToday is what the day measured at a glance, in four tiles: the
// weight (and the trend calibration smooths it to), the night's sleep,
// the steps and the resting heart rate, each a dash while the day has
// none. All its measurements, to correct, delete or add, are a sheet
// away. A day to come takes none (canLog).
export function BodyToday({
  date,
  metrics,
  canLog = true,
}: {
  date: string
  metrics: Metric[]
  canLog?: boolean
}) {
  const t = useMessages()
  const f = useFormat()
  const [open, setOpen] = useState(false)
  const trend = $api.useQuery('get', '/api/v1/summary').data?.trend
  const value = (kind: Metric['kind']) => metrics.find((m) => m.kind === kind)?.value

  const weight = value('weight')
  const sleep = value('sleep_h')
  const steps = value('steps')
  const rhr = value('rhr')
  const tiles = [
    {
      label: t.day.metric.weight,
      value: weight === undefined ? undefined : `${f.number(weight, 1)} kg`,
      note: trend === undefined ? undefined : t.day.body.trend(f.number(trend.kg, 1)),
    },
    {
      label: t.day.metric.sleep_h,
      value:
        sleep === undefined
          ? undefined
          : t.day.body.sleep(
              f.number(Math.floor(Math.round(sleep * 60) / 60)),
              f.number(Math.round(sleep * 60) % 60),
            ),
    },
    { label: t.day.metric.steps, value: steps === undefined ? undefined : f.number(steps) },
    {
      label: t.day.metric.rhr,
      value: rhr === undefined ? undefined : `${f.number(rhr)} bpm`,
    },
  ]

  return (
    <section aria-labelledby="day-body" className="space-y-3 rounded-lg border bg-card p-4">
      <div className="flex items-baseline justify-between gap-2">
        <h2 id="day-body" className="text-h3">
          {t.day.body.title}
        </h2>
        <Button
          variant="link"
          className="h-auto p-0 text-small font-semibold"
          onClick={() => {
            setOpen(true)
          }}
        >
          {t.day.body.all}
        </Button>
      </div>
      <dl className="grid grid-cols-2 gap-2">
        {tiles.map((tile) => (
          <div key={tile.label} className="space-y-0.5 rounded-md bg-surface-2 p-3">
            <dt className="text-caption text-muted-foreground">{tile.label}</dt>
            <dd className="text-h2">{tile.value ?? t.day.body.none}</dd>
            {tile.note !== undefined && <dd className="text-caption text-success">{tile.note}</dd>}
          </div>
        ))}
      </dl>
      <Sheet open={open} onOpenChange={setOpen} title={t.day.measurements}>
        <DayMetrics date={date} metrics={metrics} canLog={canLog} />
      </Sheet>
    </section>
  )
}
