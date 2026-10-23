import type { Metric } from '@/api/types'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// digits are the decimals a kind of measurement is shown with.
const digits: Partial<Record<Metric['kind'], number>> = {
  weight: 1,
  body_fat: 1,
  lean_mass: 1,
  sleep_h: 1,
  vo2max: 1,
}

// DayMetrics are the day's measurements, each in its unit.
export function DayMetrics({ metrics }: { metrics: Metric[] }) {
  const t = useMessages()
  const f = useFormat()

  return (
    <section aria-labelledby="day-measurements" className="space-y-2">
      <h2 id="day-measurements" className="font-medium">
        {t.day.measurements}
      </h2>
      {metrics.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t.day.noMeasurements}</p>
      ) : (
        <dl className="grid grid-cols-2 gap-2 text-sm">
          {metrics.map((m) => (
            <div key={m.kind} className="rounded-lg border p-3">
              <dt className="text-muted-foreground">{t.day.metric[m.kind]}</dt>
              <dd className="text-lg font-semibold tabular-nums">
                {f.number(m.value, digits[m.kind] ?? 0)}
                {/* The label says they are steps. */}
                {m.kind === 'steps' ? '' : ` ${m.unit}`}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  )
}
