import { useId } from 'react'

import { Chart } from '@/components/charts/chart'
import { Legend, type Mark } from '@/components/charts/legend'
import type { Option } from '@/components/charts/options'
import { card, entry } from '@/components/progress/styles'
import { cn } from '@/lib/utils'

// ChartCard is one of the progress's main charts: its title and a line of
// what it shows (note), its number now beside them (value, with what it
// is, caption), the legend of its marks, and the chart, which label sums
// up for a screen reader. Its tooltip stays within it.
export function ChartCard({
  title,
  note,
  value,
  caption,
  marks,
  option,
  label,
}: {
  title: string
  note?: string | undefined
  value?: string | undefined
  caption?: string | undefined
  marks: Mark[]
  option: Option
  label: string
}) {
  const id = useId()

  return (
    <section aria-labelledby={id} className={cn(card, 'min-w-0 space-y-3 overflow-hidden', entry)}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 space-y-0.5">
          <h2 id={id} className="text-h3">
            {title}
          </h2>
          {note !== undefined && <p className="text-small text-muted-foreground">{note}</p>}
        </div>
        {value !== undefined && (
          <p className="shrink-0 text-right">
            <span className="block text-h2 whitespace-nowrap">{value}</span>
            {caption !== undefined && (
              <span className="block text-caption text-muted-foreground">{caption}</span>
            )}
          </p>
        )}
      </div>
      <Legend marks={marks} />
      <Chart option={option} label={label} />
    </section>
  )
}
