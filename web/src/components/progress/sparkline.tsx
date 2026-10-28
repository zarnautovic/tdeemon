import { weekly, type Point } from '@/components/charts/options'
import { cn } from '@/lib/utils'

// The sparkline's height, in its viewBox's units, and its margin above
// and below a line, which its stroke would otherwise run over.
const height = 40
const margin = 3

// The most days a sparkline draws one by one: more are a blur in a tile.
const most = 120

// Sparkline is a measurement's days at a glance, a day to a unit across
// (a week, over many days): a line through the days that have a value,
// or a bar each (bars), from 0. Its colour is a class (className), or a
// bar's own (classOf). To a screen reader it is an image, which label
// sums up.
export function Sparkline({
  values: days,
  label,
  bars = false,
  className,
  classOf,
}: {
  values: Point[]
  label: string
  bars?: boolean
  className?: string | undefined
  classOf?: ((v: number) => string) | undefined
}) {
  const values = days.length > most ? weekly(days) : days
  const points = values.flatMap((v, i) => (v === null ? [] : [{ v, x: i + 0.5 }]))
  const all = points.map((p) => p.v)
  const lo = Math.min(...all)
  const hi = Math.max(...all)
  // A line's values span its height; one value, or all the same, is
  // drawn across its middle.
  const y = (v: number) =>
    hi === lo ? height / 2 : height - margin - ((v - lo) / (hi - lo)) * (height - 2 * margin)

  let path = points
    .map((p, i) => `${i === 0 ? 'M' : 'L'}${String(p.x)} ${String(y(p.v))}`)
    .join(' ')
  const [only] = points
  if (points.length === 1 && only !== undefined) {
    path = `M${String(only.x - 0.5)} ${String(y(only.v))} L${String(only.x + 0.5)} ${String(y(only.v))}`
  }

  return (
    <svg
      // oxlint-disable-next-line jsx-a11y/prefer-tag-over-role -- an inline <svg> takes its colours from the page's classes, which an <img> would not
      role="img"
      aria-label={label}
      viewBox={`0 0 ${String(Math.max(values.length, 1))} ${String(height)}`}
      preserveAspectRatio="none"
      className="h-10 w-full overflow-visible"
    >
      {bars ? (
        points.map((p) => {
          // A bar is from the foot, against the most; a unit high at
          // least, so a day with little shows.
          const top = height - Math.max(1, hi > 0 ? (p.v / hi) * height : 0)
          return (
            <rect
              key={p.x}
              x={p.x - 0.35}
              width={0.7}
              y={top}
              height={height - top}
              className={cn(className, classOf?.(p.v))}
            />
          )
        })
      ) : (
        <path
          d={path}
          fill="none"
          strokeWidth={2}
          strokeLinecap="round"
          strokeLinejoin="round"
          vectorEffect="non-scaling-stroke"
          className={className}
        />
      )}
    </svg>
  )
}
