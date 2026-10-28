import { useState, type ReactNode } from 'react'

import { Chart } from '@/components/charts/chart'
import type { Option, Point } from '@/components/charts/options'
import { Sparkline } from '@/components/progress/sparkline'
import { entry } from '@/components/progress/styles'
import { Sheet } from '@/components/sheet'
import { cn } from '@/lib/utils'

// MetricTile is a measurement of the days in small: its name, its number
// (value, with what it is, caption) and its days as a sparkline, which
// said sums up; legend says what its colours mean, when they mean
// anything. A tap (its name is a button as big as the tile) opens it as a
// whole chart, with its axis and a tooltip by day, in a sheet, under the
// whole chart's legend (chartLegend, else the tile's).
export function MetricTile({
  title,
  value,
  caption,
  said,
  values,
  bars = false,
  className,
  classOf,
  legend,
  chartLegend = legend,
  option,
}: {
  title: string
  value: string
  caption: string
  said: string
  values: Point[]
  bars?: boolean
  className?: string | undefined
  classOf?: ((v: number) => string) | undefined
  legend?: ReactNode
  chartLegend?: ReactNode
  option: Option
}) {
  const [open, setOpen] = useState(false)

  return (
    <li
      className={cn(
        'relative flex min-w-0 flex-col gap-2 rounded-lg border bg-card p-3 hover:bg-muted/40 has-focus-visible:ring-2 has-focus-visible:ring-ring lg:p-4',
        entry,
      )}
    >
      <h3 className="text-small text-muted-foreground">
        <button
          type="button"
          aria-haspopup="dialog"
          onClick={() => {
            setOpen(true)
          }}
          // The whole tile is the button's to tap.
          className="text-left outline-none after:absolute after:inset-0 after:rounded-lg"
        >
          {title}
        </button>
      </h3>
      <p className="flex flex-wrap items-baseline gap-x-1.5">
        <span className="text-h2">{value}</span>
        <span className="text-caption text-muted-foreground">{caption}</span>
      </p>
      <Sparkline values={values} label={said} bars={bars} className={className} classOf={classOf} />
      {legend}
      <Sheet open={open} onOpenChange={setOpen} title={title}>
        <div className="space-y-3">
          <p className="text-small text-muted-foreground">{said}</p>
          {chartLegend}
          <Chart option={option} label={said} height={260} />
        </div>
      </Sheet>
    </li>
  )
}
