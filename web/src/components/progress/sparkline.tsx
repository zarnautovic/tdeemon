import { average7, weekly, type Point } from '@/components/charts/options'

// The sparkline's height, in its viewBox's units, and its margin above
// and below a line, which its stroke would otherwise run over.
const height = 40
const margin = 3

// The most days a sparkline draws one by one: more are a blur in a tile.
const most = 120

// Sparkline is a measurement's days at a glance: their 7 days' average, a
// day to a unit across (a week, over many days), as a line in the charts'
// neutral colour, over its usual range (band) shaded when it has one. It
// means nothing by its colour, and draws what its tile says in words, so
// a screen reader skips it.
export function Sparkline({
  values: days,
  band,
}: {
  values: Point[]
  band?: { low: number; high: number } | undefined
}) {
  const averages = average7(days)
  const values = averages.length > most ? weekly(averages) : averages
  const points = values.flatMap((v, i) => (v === null ? [] : [{ v, x: i + 0.5 }]))
  const all = [...points.map((p) => p.v), ...(band ? [band.low, band.high] : [])]
  const lo = Math.min(...all)
  const hi = Math.max(...all)
  // A line's values span its height; one value, or all the same, is
  // drawn across its middle.
  const y = (v: number) =>
    hi === lo ? height / 2 : height - margin - ((v - lo) / (hi - lo)) * (height - 2 * margin)
  const width = Math.max(values.length, 1)

  let path = points
    .map((p, i) => `${i === 0 ? 'M' : 'L'}${String(p.x)} ${String(y(p.v))}`)
    .join(' ')
  const [only] = points
  if (points.length === 1 && only !== undefined) {
    path = `M${String(only.x - 0.5)} ${String(y(only.v))} L${String(only.x + 0.5)} ${String(y(only.v))}`
  }

  return (
    <svg
      aria-hidden
      data-testid="sparkline"
      viewBox={`0 0 ${String(width)} ${String(height)}`}
      preserveAspectRatio="none"
      className="h-10 w-full overflow-visible"
    >
      {band && (
        <rect
          data-testid="usual"
          x={0}
          width={width}
          y={y(band.high)}
          height={y(band.low) - y(band.high)}
          className="fill-chart-5/20"
        />
      )}
      <path
        d={path}
        fill="none"
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
        className="stroke-chart-5"
      />
    </svg>
  )
}
