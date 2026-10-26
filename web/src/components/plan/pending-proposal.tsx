import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useId } from 'react'

import { $api } from '@/api/client'
import { statusOf } from '@/api/errors'
import { sessionQuery } from '@/api/session'
import {
  PreviewLines,
  ProposalChanges,
  type Preview,
  type Proposal,
} from '@/components/plan/proposal-changes'
import { useViolationText, violationsOf } from '@/components/plan/proposal-text'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// PendingProposal is the proposal of a change of the plan that waits for
// the user's answer, as a model made it in a chat that did not decide it:
// why, what it changes, what it would come to by the data now, and the
// buttons to accept or reject it. An accepted one changes the plan, so
// every answer the app holds is read again.
export function PendingProposal({
  proposal,
  preview,
}: {
  proposal: Proposal
  preview: Preview | undefined
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const queryClient = useQueryClient()
  const violation = useViolationText()
  const done = () => queryClient.invalidateQueries()
  const accept = $api.useMutation('post', '/api/v1/proposals/{id}/apply', { onSuccess: done })
  const reject = $api.useMutation('post', '/api/v1/proposals/{id}/reject', { onSuccess: done })
  const path = { params: { path: { id: proposal.id } } }
  const error = accept.error ?? reject.error
  const failed = accept.isError || reject.isError
  // 409: it waits no more (applied or rejected elsewhere, superseded,
  // expired); 422: the limits refuse it now, or a day of it went by.
  const gone = statusOf(error) === 409
  const refused = violationsOf(error)
  const busy = accept.isPending || reject.isPending
  const p = t.plan.pending

  return (
    <section aria-labelledby={id} className="space-y-3 rounded-xl border p-4">
      <h2 id={id} className="text-lg font-semibold">
        {p.title}
      </h2>
      <p className="text-sm text-muted-foreground">
        {p.by(proposal.by.model ?? proposal.by.client)}
      </p>
      <p className="whitespace-pre-line">{proposal.rationale}</p>
      {proposal.window !== undefined && (
        <p className="text-sm text-muted-foreground">
          {p.window(f.short(proposal.window.from), f.short(proposal.window.to))}
        </p>
      )}
      <ProposalChanges changes={proposal.changes} />
      {preview !== undefined && <PreviewLines preview={preview} />}
      <p className="text-sm text-muted-foreground">
        {p.expires(f.day(proposal.expires_at, timeZone))}
      </p>
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
          ) : (
            <p>{p.failed}</p>
          )}
        </div>
      )}
      <div className="flex gap-2">
        <Button
          disabled={busy || gone}
          onClick={() => {
            accept.mutate(path)
          }}
        >
          {accept.isPending ? p.sending : p.accept}
        </Button>
        <Button
          variant="outline"
          disabled={busy || gone}
          onClick={() => {
            reject.mutate(path)
          }}
        >
          {p.reject}
        </Button>
      </div>
    </section>
  )
}
