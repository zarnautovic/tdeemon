import { Link } from '@tanstack/react-router'
import { Gauge } from 'lucide-react'

import { $api } from '@/api/client'
import { useReviewWords } from '@/components/autopilot/review-words'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// AutopilotLine is this morning's weekly review on today, in a line under
// the day's hero, a link to the plan, where it is said whole: a step
// applied in the brand's colours (the target above it changed), a hold or
// no step plain. A proposal it made is the proposal's line (ProposalNudge)
// instead, which asks for an answer. Only on the review's day: the
// morning summary and the plan say it after.
export function AutopilotLine({ today }: { today: string }) {
  const t = useMessages()
  const say = useReviewWords()
  // Asked as the day's page is (useDayReady): not again for a failure.
  const autopilot = $api.useQuery('get', '/api/v1/autopilot', undefined, { retryOnMount: false })
  const last = autopilot.data?.last
  if (last === undefined || last.date !== today || last.outcome === 'proposed') {
    return null
  }
  const applied = last.outcome === 'applied'

  return (
    <Link
      to="/plan"
      className={cn(
        'flex min-h-11 items-center gap-2 rounded-md border px-3 py-2 text-small outline-none focus-visible:ring-2 focus-visible:ring-ring',
        applied
          ? 'border-primary/30 bg-primary-soft text-primary'
          : 'bg-card text-muted-foreground',
      )}
    >
      <Gauge aria-hidden className="size-4 shrink-0" />
      <span className="min-w-0 flex-1 truncate">{t.autopilot.line(say(last).line)}</span>
      <span className={cn('shrink-0 font-semibold', !applied && 'text-foreground')}>
        {t.autopilot.why}
      </span>
    </Link>
  )
}
