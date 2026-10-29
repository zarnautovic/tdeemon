import { useRef, type Ref } from 'react'

import type { Metric } from '@/api/types'
import { AddMetric } from '@/components/day/add-metric'
import { MetricCard } from '@/components/day/metric-card'
import { MetricForm } from '@/components/day/metric-form'
import { metricKinds, type MetricKind } from '@/components/day/metric-kinds'
import { useMessages } from '@/i18n/messages'

// DayMetrics are the measurements of day date, each in its unit, to
// correct or delete; a field for the weight while the day has none, the
// one given most, and the others to add. A day to come takes none
// (canLog). The sheet it is in gives its title. Opened at a kind (a tile
// of the body's), the field of its measurement is open, the one to
// correct or the one to add, and field is its, for the sheet to focus. A
// measurement deleted takes the focus with its card: the measurements
// take it, so it stays in the sheet, and F6 still reaches the toast's
// Undo.
export function DayMetrics({
  date,
  metrics,
  canLog = true,
  at,
  field,
}: {
  date: string
  metrics: Metric[]
  canLog?: boolean
  at?: MetricKind | undefined
  field?: Ref<HTMLInputElement> | undefined
}) {
  const t = useMessages()
  const has = new Set(metrics.map((m) => m.kind))
  const others = metricKinds.filter((k) => k !== 'weight' && !has.has(k))
  const root = useRef<HTMLDivElement>(null)
  // The field the sheet is open at, of the kind it is open at.
  const fieldOf = (kind: MetricKind) => (canLog && at === kind ? field : undefined)

  return (
    <div ref={root} tabIndex={-1} className="space-y-2 outline-none">
      {canLog && !has.has('weight') && (
        <div className="rounded-lg border p-3">
          <MetricForm
            date={date}
            kind="weight"
            label={t.day.metric.weight}
            inputRef={fieldOf('weight')}
          />
        </div>
      )}
      {metrics.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t.day.noMeasurements}</p>
      ) : (
        <ul className="grid grid-cols-2 gap-2">
          {metrics.map((m) => (
            <MetricCard
              key={m.kind}
              date={date}
              metric={m}
              canLog={canLog}
              open={canLog && at === m.kind}
              inputRef={fieldOf(m.kind)}
              onDeleted={() => {
                root.current?.focus()
              }}
            />
          ))}
        </ul>
      )}
      {canLog &&
        (at !== undefined && others.includes(at) ? (
          <AddMetric date={date} kinds={others} at={at} inputRef={field} />
        ) : (
          <AddMetric date={date} kinds={others} />
        ))}
    </div>
  )
}
