import { Pencil } from 'lucide-react'
import { useState } from 'react'

import { useDeleteMetric } from '@/api/day'
import type { Metric } from '@/api/types'
import { MetricForm } from '@/components/day/metric-form'
import { digits } from '@/components/day/metric-kinds'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// MetricCard is a measurement of the day in its unit, which can be
// corrected or deleted.
export function MetricCard({ date, metric }: { date: string; metric: Metric }) {
  const t = useMessages()
  const f = useFormat()
  const [editing, setEditing] = useState(false)
  const remove = useDeleteMetric(date)
  const label = t.day.metric[metric.kind]

  if (editing) {
    return (
      <li className="col-span-2 space-y-1 rounded-lg border p-3">
        <MetricForm
          date={date}
          kind={metric.kind}
          label={label}
          initial={f.field(metric.value)}
          onSaved={() => {
            setEditing(false)
          }}
        >
          <Button
            variant="destructive"
            size="lg"
            className="h-10"
            disabled={remove.isPending}
            onClick={() => {
              remove.mutate({ params: { path: { date, kind: metric.kind } } })
            }}
          >
            {t.day.delete}
          </Button>
          <Button
            variant="ghost"
            size="lg"
            className="h-10"
            onClick={() => {
              setEditing(false)
            }}
          >
            {t.day.cancel}
          </Button>
        </MetricForm>
        {remove.isError && <SaveStatus change={remove} />}
      </li>
    )
  }

  // The value in its unit, as the language writes it: the steps without
  // one, which the label says.
  let value = f.number(metric.value, digits[metric.kind] ?? 0)
  if (metric.unit === '%') {
    value = f.percent(metric.value)
  } else if (metric.kind !== 'steps') {
    value = `${value} ${metric.unit}`
  }

  return (
    <li className="relative rounded-lg border p-3 text-sm">
      <p className="text-muted-foreground">{label}</p>
      <p className="text-lg font-semibold tabular-nums">{value}</p>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t.day.change(label)}
        className="absolute top-2 right-2"
        onClick={() => {
          setEditing(true)
        }}
      >
        <Pencil aria-hidden />
      </Button>
    </li>
  )
}
