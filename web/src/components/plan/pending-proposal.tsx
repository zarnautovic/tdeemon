import { useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'

import { $api } from '@/api/client'
import { statusOf } from '@/api/errors'
import type { components } from '@/api/schema'
import { useRationale, useWho } from '@/components/plan/change-words'
import { Loosen } from '@/components/plan/loosen'
import {
  PreviewLines,
  ProposalChanges,
  type Preview,
  type Proposal,
} from '@/components/plan/proposal-changes'
import { ProposalData } from '@/components/plan/proposal-data'
import {
  useDate,
  useLimitWords,
  useViolationText,
  violationsOf,
} from '@/components/plan/proposal-text'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

type Plan = components['schemas']['PlanBody']

// PendingProposal is the proposal of a change of the plan that waits for
// the user's answer, as a model made it in a chat that did not decide it:
// who made it and when, how long it waits, why, what it changes against
// the plan (plan), what the data say beside it, what it would come to by
// the data now, and the buttons to accept or reject it. An accepted one
// changes the plan, so every answer the app holds is read again. One that
// changes the user's limits is accepted only here, and one that loosens a
// limit asks first, as the limits' form does.
export function PendingProposal({
  proposal,
  preview,
  plan,
}: {
  proposal: Proposal
  preview: Preview | undefined
  plan: Plan
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const queryClient = useQueryClient()
  const violation = useViolationText()
  const limit = useLimitWords()
  const date = useDate()
  const who = useWho()
  const rationale = useRationale()
  // When the card came, for how long ago it was made and how long it
  // waits: a proposal waits days, so a minute does not matter.
  const [now] = useState(Date.now)
  const daysLeft = Math.max(
    1,
    Math.ceil((new Date(proposal.expires_at).getTime() - now) / 86_400_000),
  )
  // Whether the user is asked to loosen what the proposal loosens.
  const [asking, setAsking] = useState(false)
  const loosened = proposal.changes.limits.filter((ch) => ch.loosens)
  const done = () => queryClient.invalidateQueries()
  const accept = $api.useMutation('post', '/api/v1/proposals/{id}/apply', {
    onSuccess: done,
    onError: () => {
      setAsking(false)
    },
  })
  const reject = $api.useMutation('post', '/api/v1/proposals/{id}/reject', { onSuccess: done })
  const path = { params: { path: { id: proposal.id } } }
  // Each action starts by forgetting the other's answer, so the one shown
  // is the latest.
  const error = accept.error ?? reject.error
  const failed = accept.isError || reject.isError
  // 409: it waits no more (applied or rejected elsewhere, superseded,
  // expired); 422: the limits refuse it now, or, without a violation, a
  // day of it went by and it can never apply, though it may be rejected.
  const gone = statusOf(error) === 409
  const refused = violationsOf(error)
  const stale = statusOf(error) === 422 && refused.length === 0
  const busy = accept.isPending || reject.isPending
  const p = t.plan.pending

  return (
    <section
      aria-labelledby={id}
      className="space-y-3 rounded-lg border border-primary/40 bg-card p-4 shadow-card lg:p-5"
    >
      <header className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0 space-y-0.5">
          <h2 id={id} className="text-h3">
            {p.title}
          </h2>
          <p className="text-sm text-muted-foreground">
            {[
              proposal.by.client === 'web' && proposal.by.model === undefined
                ? p.byYou
                : p.by(who(proposal.by)),
              f.ago(proposal.created_at, now),
            ].join(' · ')}
          </p>
        </div>
        <span className="shrink-0 rounded-full bg-warning/10 px-3 py-1 text-small text-warning">
          {p.expiresIn(daysLeft)}
        </span>
      </header>
      <p className="whitespace-pre-line">{rationale(proposal.rationale)}</p>
      {proposal.window !== undefined && (
        <p className="text-sm text-muted-foreground">
          {p.window(date(proposal.window.from), date(proposal.window.to))}
        </p>
      )}
      <ProposalChanges changes={proposal.changes} targets={plan.targets} />
      {proposal.changes.limits.length > 0 && (
        <p className="text-sm text-muted-foreground">{p.limitsHere}</p>
      )}
      <ProposalData proposal={proposal} plan={plan} />
      {preview !== undefined && <PreviewLines preview={preview} />}
      {failed && (
        <div role="alert" className="space-y-1 text-sm text-destructive">
          {gone ? (
            <p>{p.gone}</p>
          ) : refused.length > 0 ? (
            <>
              <p>{p.refusedNow}</p>
              <ul>
                {refused.map((v) => (
                  <li key={`${v.rule} ${v.day_type ?? ''}`}>{violation(v)}</li>
                ))}
              </ul>
            </>
          ) : stale ? (
            <p>{p.stale}</p>
          ) : (
            <p>{p.failed}</p>
          )}
        </div>
      )}
      {asking && (
        <Loosen
          names={loosened.map((ch) => limit.label(ch.name)).join(', ')}
          pending={busy}
          onYes={() => {
            accept.mutate({ ...path, body: { confirm: true } })
          }}
          onBack={() => {
            setAsking(false)
          }}
        />
      )}
      <div className="flex gap-2">
        <Button
          disabled={busy || gone || stale || asking}
          onClick={() => {
            reject.reset()
            if (loosened.length > 0) {
              accept.reset()
              setAsking(true)
              return
            }
            accept.mutate(path)
          }}
        >
          {accept.isPending ? p.sending : p.accept}
        </Button>
        <Button
          variant="outline"
          disabled={busy || gone}
          onClick={() => {
            accept.reset()
            setAsking(false)
            reject.mutate(path)
          }}
        >
          {p.reject}
        </Button>
      </div>
    </section>
  )
}
