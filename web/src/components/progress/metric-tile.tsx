import { ArrowDown, ArrowUp, type LucideIcon } from 'lucide-react'
import { useId, useState, type ReactNode } from 'react'

import { Chart } from '@/components/charts/chart'
import type { Option, Point } from '@/components/charts/options'
import { Sparkline } from '@/components/progress/sparkline'
import { entry } from '@/components/progress/styles'
import { Sheet } from '@/components/sheet'
import { cn } from '@/lib/utils'

// Usual is how the tile's number stands against the usual: in words
// (said), which way it went, and whether that is worse, as recovery
// flags it.
export interface Usual {
  said: string
  way: 'up' | 'down' | 'same'
  worse: boolean
}

const arrows: Record<Usual['way'], LucideIcon | undefined> = {
  up: ArrowUp,
  down: ArrowDown,
  same: undefined,
}

// MetricTile is a measurement in small, the whole tile one button: its
// name, its number (value, with what it is, caption), how that stands
// against the usual (usual), and its days' 7 days' average as a
// sparkline, over the usual range (band). A tap opens it as a whole
// chart, with its axis and a tooltip by day, in a sheet, under its
// legend; said sums that chart up, and describes the tile to a screen
// reader.
export function MetricTile({
  title,
  value,
  caption,
  usual,
  said,
  values,
  band,
  legend,
  option,
}: {
  title: string
  value: string
  caption: string
  usual?: Usual | undefined
  said: string
  values: Point[]
  band?: { low: number; high: number } | undefined
  legend?: ReactNode
  option: Option
}) {
  const [open, setOpen] = useState(false)
  const saidId = useId()
  const Arrow = usual === undefined ? undefined : arrows[usual.way]

  return (
    <li className="min-w-0">
      <button
        type="button"
        aria-haspopup="dialog"
        aria-describedby={saidId}
        onClick={() => {
          setOpen(true)
        }}
        className={cn(
          'flex size-full min-h-11 min-w-0 flex-col gap-2 rounded-lg border bg-card p-3 text-left outline-none hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring lg:p-4',
          entry,
        )}
      >
        <span className="text-small text-muted-foreground">{title}</span>
        <span className="flex flex-wrap items-baseline gap-x-1.5">
          <span className="text-h2">{value}</span>
          <span className="text-caption text-muted-foreground">{caption}</span>
        </span>
        {usual !== undefined && (
          <span
            className={cn(
              'flex items-center gap-1 text-caption',
              usual.worse ? 'text-warning' : 'text-muted-foreground',
            )}
          >
            {Arrow !== undefined && <Arrow aria-hidden className="size-3.5 shrink-0" />}
            {usual.said}
          </span>
        )}
        <Sparkline values={values} band={band} />
      </button>
      <span id={saidId} className="sr-only">
        {said}
      </span>
      <Sheet open={open} onOpenChange={setOpen} title={title}>
        <div className="space-y-3">
          <p className="text-small text-muted-foreground">{said}</p>
          {legend}
          <Chart option={option} label={said} height={260} />
        </div>
      </Sheet>
    </li>
  )
}
