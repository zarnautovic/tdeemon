import { cn } from '@/lib/utils'

// A mark is how a series is drawn: a line, a dashed line, a band, dots or
// bars. Its colour is a class of the token the series is drawn in: bg-…,
// or border-… of a dashed line.
export interface Mark {
  name: string
  mark: 'line' | 'dashed' | 'band' | 'dot' | 'bar'
  color: string
}

// The marks' shapes, as small as the legend's text.
const shapes: Record<Mark['mark'], string> = {
  line: 'h-0.5 w-4 rounded-full',
  dashed: 'w-4 border-t-2 border-dashed',
  band: 'h-2.5 w-4 rounded-xs opacity-25',
  dot: 'size-2 rounded-full',
  bar: 'size-2 rounded-xs',
}

// Legend says what a chart's colours and marks mean, each with its name:
// HTML, which wraps on a phone, where ECharts' own would run into the
// chart. A chart's summary says it to a screen reader already.
export function Legend({ marks }: { marks: Mark[] }) {
  return (
    <ul aria-hidden className="flex flex-wrap gap-x-3 gap-y-1 text-caption text-muted-foreground">
      {marks.map((m) => (
        <li key={m.name} className="flex items-center gap-1.5">
          <span className={cn('shrink-0', shapes[m.mark], m.color)} />
          {m.name}
        </li>
      ))}
    </ul>
  )
}
