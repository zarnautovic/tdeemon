import type { Metric } from '@/api/types'
import { AddMetric } from '@/components/day/add-metric'
import { MetricCard } from '@/components/day/metric-card'
import { MetricForm } from '@/components/day/metric-form'
import { metricKinds } from '@/components/day/metric-kinds'
import { useMessages } from '@/i18n/messages'

// DayMetrics are the measurements of day date, each in its unit, to
// correct or delete; a field for the weight while the day has none, the
// one given most, and the others to add. A day to come takes none
// (canLog). The sheet it is in gives its title.
export function DayMetrics({
  date,
  metrics,
  canLog = true,
}: {
  date: string
  metrics: Metric[]
  canLog?: boolean
}) {
  const t = useMessages()
  const has = new Set(metrics.map((m) => m.kind))
  const others = metricKinds.filter((k) => k !== 'weight' && !has.has(k))

  return (
    <div className="space-y-2">
      {canLog && !has.has('weight') && (
        <div className="rounded-lg border p-3">
          <MetricForm date={date} kind="weight" label={t.day.metric.weight} />
        </div>
      )}
      {metrics.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t.day.noMeasurements}</p>
      ) : (
        <ul className="grid grid-cols-2 gap-2">
          {metrics.map((m) => (
            <MetricCard key={m.kind} date={date} metric={m} canLog={canLog} />
          ))}
        </ul>
      )}
      {canLog && <AddMetric date={date} kinds={others} />}
    </div>
  )
}
