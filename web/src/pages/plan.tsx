import { $api } from '@/api/client'
import { AutopilotCard } from '@/components/autopilot/autopilot-card'
import { LoadError } from '@/components/load-error'
import { NoPlan } from '@/components/plan/no-plan'
import { PendingProposal } from '@/components/plan/pending-proposal'
import { PhaseCard } from '@/components/plan/phase-card'
import { Revisions } from '@/components/plan/revisions'
import { TargetsCard } from '@/components/plan/targets-card'
import { YouList } from '@/components/plan/you-list'
import { SettingsList } from '@/components/settings/settings-list'
import { Skeleton } from '@/components/skeleton'
import { useMessages } from '@/i18n/messages'

// PlanPage is the user's plan: its phase, the hero, with the phases on a
// timeline; a change a coach proposed, which waits for the user's answer;
// the autopilot, its weekly review and the next, above the targets it
// moves; the targets by type of day against the TDEE; and the history of the
// plan's changes. Before a plan, how one comes. Beside it on a desktop,
// under it on a phone, what a plan rests on of the user's (the profile,
// the limits, the notes for a coach), each a row to its page; and on a
// phone, whose bar has no room for them, the settings.
export function PlanPage() {
  const t = useMessages()
  const plan = $api.useQuery('get', '/api/v1/plan')
  const me = $api.useQuery('get', '/api/v1/me')
  // The autopilot's card comes with the plan, not after it, so nothing
  // under it moves when it does; once it failed, the plan says it without
  // waiting for it again.
  const autopilot = $api.useQuery('get', '/api/v1/autopilot')
  const autopilotWaits = autopilot.isLoading && autopilot.errorUpdateCount === 0

  // A plan that came stays while asking for it again fails: the bar over
  // the page says since when.
  let body
  if (plan.data === undefined || autopilotWaits) {
    body =
      plan.isError && plan.data === undefined ? (
        <LoadError
          title={t.loadFailed.plan}
          error={plan.error}
          onRetry={() => void plan.refetch()}
        />
      ) : (
        <Skeleton blocks={['h-80', 'h-52']} />
      )
  } else {
    const { phases, targets, training_weekdays: days, pending, revisions } = plan.data
    const none = phases.length === 0 && targets.length === 0
    body = (
      <>
        {/* A proposal is the first step to a plan, when one waits. */}
        {none && pending === undefined && (
          <NoPlan needsProfile={(me.data?.profile.missing.length ?? 0) > 0} />
        )}
        {phases.length > 0 && <PhaseCard plan={plan.data} />}
        {pending !== undefined && (
          // A proposal in place of another starts afresh, without the
          // other's answer.
          <PendingProposal
            key={pending.id}
            proposal={pending}
            preview={plan.data.pending_preview}
            plan={plan.data}
          />
        )}
        {/* The autopilot, above the targets it moves, once there is a
            plan to review. */}
        {(phases.length > 0 || targets.length > 0) && (
          <AutopilotCard plan={plan.data} autopilot={autopilot.data} />
        )}
        {(targets.length > 0 || days.length > 0 || phases.length > 0) && (
          <TargetsCard plan={plan.data} />
        )}
        {revisions.length > 0 && <Revisions revisions={revisions} targets={targets} />}
      </>
    )
  }

  return (
    <div className="space-y-4">
      <h1 className="text-title">{t.plan.title}</h1>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-12 lg:items-start">
        <div className="space-y-3 lg:col-span-8 lg:space-y-4">{body}</div>
        <div className="space-y-6 lg:sticky lg:top-8 lg:col-span-4">
          <YouList />
          <SettingsList titled className="lg:hidden" />
        </div>
      </div>
    </div>
  )
}
