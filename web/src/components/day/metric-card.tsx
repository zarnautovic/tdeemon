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
import { sayChange } from '@/lib/toast'

// MetricCard is a measurement of the day in its unit, which can be
// corrected or deleted, but on a day to come (canLog false); a toast says
// so, with Undo, and no question before. onDeleted is told once it is
// deleted, as its card goes with the focus in it.
export function MetricCard({
  date,
  metric,
  canLog = true,
  onDeleted,
}: {
  date: string
  metric: Metric
  canLog?: boolean
  onDeleted?: () => void
}) {
  const t = useMessages()
  const f = useFormat()
  const [editing, setEditing] = useState(false)
  const label = t.day.metric[metric.kind]
  const remove = useDeleteMetric(date, () => {
    sayChange(
      { title: t.toast.deleted(label) },
      { last: { action: 'metric.delete', entity: `${date}/${metric.kind}` }, date },
    )
    onDeleted?.()
  })

  if (editing) {
    return (
      <li className="col-span-2 space-y-1 rounded-lg border p-3">
        <MetricForm
          date={date}
          kind={metric.kind}
          label={label}
          // As the card shows it: steps whole, a weight to 0.1.
          initial={f.field(metric.value, digits[metric.kind] ?? 0)}
          onSaved={() => {
            setEditing(false)
          }}
        >
          <Button
            variant="destructive"
            disabled={remove.isPending}
            onClick={() => {
              remove.mutate({ params: { path: { date, kind: metric.kind } } })
            }}
          >
            {t.day.delete}
          </Button>
          <Button
            variant="ghost"
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
      {canLog && (
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
      )}
    </li>
  )
}
