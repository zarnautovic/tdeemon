import { Check } from 'lucide-react'

import type { components } from '@/api/schema'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

type Needs = components['schemas']['TdeeNeedsBody']

// met reports whether a window has what it needs of something.
function met(n: { has: number; need: number }): boolean {
  return n.has >= n.need
}

// CalibrationCounts are what calibration has toward a first TDEE against
// what it needs, both the summary's (tdee.needs, DESIGN.md §7), each
// ticked once it has enough: the days, the logged days and the
// weigh-ins, these enough only with days enough from the first to the
// last, which are said while they are not. compact is a line of them, as
// today's hero has it; else a list.
export function CalibrationCounts({ needs, compact = false }: { needs: Needs; compact?: boolean }) {
  const t = useMessages()
  const f = useFormat()
  const c = t.calibration
  const apart = needs.weigh_in_days
  const counts = [
    { key: 'days', name: c.days, count: needs.days, met: met(needs.days) },
    { key: 'logged', name: c.loggedDays, count: needs.logged_days, met: met(needs.logged_days) },
    {
      key: 'weighIns',
      name: c.weighIns,
      count: needs.weigh_ins,
      met: met(needs.weigh_ins) && met(apart),
      // Weigh-ins enough in number, but too close together.
      note:
        met(needs.weigh_ins) && !met(apart)
          ? c.apart(f.number(apart.has), f.number(apart.need))
          : undefined,
    },
  ]
  const of = compact ? c.ofShort : c.of

  return (
    <ul className={cn(compact ? 'flex flex-wrap gap-x-3 gap-y-0.5 text-caption' : 'space-y-1.5')}>
      {counts.map((n) => (
        <li key={n.key} className="flex flex-wrap items-center gap-x-1.5">
          <span className="text-muted-foreground">{n.name}</span>
          <span className="font-semibold tabular-nums">
            {of(f.number(n.count.has), f.number(n.count.need))}
          </span>
          {n.met && (
            <>
              <Check aria-hidden className="size-4 shrink-0 text-success" />
              <span className="sr-only">{c.met}</span>
            </>
          )}
          {n.note !== undefined && (
            <span className="text-small text-muted-foreground">{n.note}</span>
          )}
        </li>
      ))}
    </ul>
  )
}
