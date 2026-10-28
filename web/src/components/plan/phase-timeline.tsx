import { useId } from 'react'

import type { Span } from '@/components/plan/phases'
import { useDate } from '@/components/plan/proposal-text'
import { Badge } from '@/components/ui/badge'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// The dot of a phase on the line: filled for one past, the brand's for
// today's, hollow for one to come. Its badge says which.
const dots = {
  past: 'bg-muted-foreground/60',
  now: 'bg-primary ring-4 ring-primary-soft',
  planned: 'border-2 border-muted-foreground bg-card',
} as const

// PhaseTimeline is the plan's phases in order, those of the last year and
// those to come: each with its type, rate and days, and whether it is
// past, today's or planned.
export function PhaseTimeline({ spans }: { spans: readonly Span[] }) {
  const t = useMessages()
  const f = useFormat()
  const date = useDate()
  const id = useId()
  const p = t.plan.phase

  return (
    <div className="space-y-2 border-t pt-4">
      <h3 id={id} className="text-small text-muted-foreground">
        {p.timeline}
      </h3>
      <ol aria-labelledby={id}>
        {spans.map((span, i) => (
          <li key={span.phase.start} className="flex gap-3">
            <span aria-hidden className="flex w-3 shrink-0 flex-col items-center">
              <span className={cn('mt-1.5 size-2.5 shrink-0 rounded-full', dots[span.state])} />
              {i < spans.length - 1 && <span className="my-1 w-px flex-1 bg-border" />}
            </span>
            <span className="min-w-0 flex-1 pb-3 text-small">
              <span
                className={cn('font-semibold', span.state === 'past' && 'text-muted-foreground')}
              >
                {t.plan.change.types[span.phase.type]}
              </span>
              <span className="text-muted-foreground">
                {' · '}
                {t.plan.change.rate(f.percent(span.phase.rate_pct_week, 2))}
              </span>
              <span className="block text-caption text-muted-foreground">
                {span.last === undefined
                  ? p.from(date(span.phase.start))
                  : p.span(date(span.phase.start), date(span.last))}
              </span>
            </span>
            <Badge tone={span.state === 'now' ? 'primary' : 'neutral'} className="self-start">
              {p.states[span.state]}
            </Badge>
          </li>
        ))}
      </ol>
    </div>
  )
}
