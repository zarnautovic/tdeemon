import { Link } from '@tanstack/react-router'
import { Sparkles } from 'lucide-react'

import { $api } from '@/api/client'
import { useGist } from '@/components/plan/change-words'
import { useMessages } from '@/i18n/messages'

// ProposalNudge is a line that a proposal waits for the user's answer,
// with its gist ("Your AI coach suggests: rest days 1,950 → 1,850 kcal"),
// a link to the plan, where it is answered; nothing while none waits. A
// proposal waits a week, then expires: the day says so, not only the plan.
export function ProposalNudge() {
  const t = useMessages()
  const gist = useGist()
  const plan = $api.useQuery('get', '/api/v1/plan')
  const pending = plan.data?.pending
  if (plan.data === undefined || pending === undefined) {
    return null
  }
  const n = t.plan.nudge
  const what = gist(pending, plan.data.targets)
  const yours = pending.by.client === 'web' && pending.by.model === undefined

  return (
    <Link
      to="/plan"
      className="flex min-h-11 items-center gap-2 rounded-md border border-primary/30 bg-primary-soft px-3 py-2 text-small text-primary outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <Sparkles aria-hidden className="size-4 shrink-0" />
      <span className="min-w-0 flex-1 truncate">{yours ? n.yours(what) : n.coach(what)}</span>
      <span className="shrink-0 font-semibold">{n.review}</span>
    </Link>
  )
}
