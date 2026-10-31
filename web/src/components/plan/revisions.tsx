import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'

import { $api } from '@/api/client'
import { statusOf } from '@/api/errors'
import type { components } from '@/api/schema'
import { sessionQuery } from '@/api/session'
import { Rationale } from '@/components/autopilot/rationale'
import { useWho } from '@/components/plan/change-words'
import { ProposalChanges, type Proposal } from '@/components/plan/proposal-changes'
import { useLimitWords, useViolationText, violationsOf } from '@/components/plan/proposal-text'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

type Target = components['schemas']['TargetPeriodBody']

// tightened are the limits a revision tightened, which taking it back
// loosens.
function tightened(rev: Proposal): string[] {
  return rev.changes.limits.filter((ch) => !ch.loosens).map((ch) => ch.name)
}

// takenBack is the code the app proposes taking a revision back with, as
// its rationale: each language says it its own way (rationales).
const takenBack = 'taken back in the app'

// Revisions is the history of the plan: the proposals applied, newest
// first, each with when, who, why and what it changed, against the plan's
// targets. One applied may be taken back in a step: the user, here,
// consents to the inverse the server makes, which applies at once. Taking
// back a limit it tightened loosens it: the question says so, and the
// answer confirms it.
export function Revisions({
  revisions,
  targets,
}: {
  revisions: Proposal[]
  targets: readonly Target[]
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const queryClient = useQueryClient()
  const violation = useViolationText()
  const limit = useLimitWords()
  const who = useWho()
  // The revision the user is asked about, and the one being taken back.
  const [asking, setAsking] = useState<string | undefined>(undefined)
  const [failed, setFailed] = useState<{ id: string; error: unknown } | undefined>(undefined)
  const revert = $api.useMutation('post', '/api/v1/proposals/{id}/revert')
  const apply = $api.useMutation('post', '/api/v1/proposals/{id}/apply')
  const busy = revert.isPending || apply.isPending
  const r = t.plan.revisions

  async function takeBack(rev: Proposal) {
    setFailed(undefined)
    try {
      const made = await revert.mutateAsync({
        params: { path: { id: rev.id } },
        body: { rationale: takenBack },
      })
      // The user confirmed what the question said it loosens, and no more:
      // anything else is asked when the revert, waiting, is accepted.
      const loosens = made.proposal.changes.limits.filter((ch) => ch.loosens)
      const told = tightened(rev)
      const confirm = loosens.length > 0 && loosens.every((ch) => told.includes(ch.name))
      await apply.mutateAsync({
        params: { path: { id: made.proposal.id } },
        ...(confirm ? { body: { confirm: true } } : {}),
      })
    } catch (error) {
      setFailed({ id: rev.id, error })
    } finally {
      setAsking(undefined)
      // A revert made but refused when applied waits, as any proposal.
      await queryClient.invalidateQueries()
    }
  }

  // why says why a change could not be taken back: the limits now, or
  // that it took effect or the plan changed since.
  const why = (error: unknown) => {
    const refused = violationsOf(error)
    if (refused.length > 0) {
      return [t.plan.pending.refusedNow, ...refused.map(violation)].join(' ')
    }
    return statusOf(error) === 422 ? r.cannot : t.plan.pending.failed
  }

  return (
    <section aria-labelledby={id} id="plan-history" className="scroll-mt-4 space-y-3">
      <h2 id={id} className="text-h3">
        {r.title}
      </h2>
      {revisions.length === 0 && <p className="text-sm text-muted-foreground">{r.none}</p>}
      <ol className="space-y-3">
        {revisions.map((rev) => (
          <li key={rev.id} className="space-y-2 rounded-lg border bg-card p-4 lg:p-5">
            <p className="text-sm text-muted-foreground">
              {r.applied(
                rev.decided_at === undefined ? '' : f.day(rev.decided_at, timeZone),
                who(rev.by),
              )}
              {rev.status === 'reverted' && ` · ${r.reverted}`}
            </p>
            <Rationale proposal={rev} brief />
            <ProposalChanges changes={rev.changes} targets={targets} />
            {failed?.id === rev.id && (
              <p role="alert" className="text-sm text-destructive">
                {why(failed.error)}
              </p>
            )}
            {rev.status === 'applied' &&
              (asking === rev.id ? (
                <div className="space-y-2">
                  <p className="text-sm">{r.confirm}</p>
                  {tightened(rev).length > 0 && (
                    <p className="text-sm">
                      {r.loosens(tightened(rev).map(limit.label).join(', '))}
                    </p>
                  )}
                  <div className="flex gap-2">
                    <Button disabled={busy} onClick={() => void takeBack(rev)}>
                      {busy ? t.plan.pending.sending : r.yes}
                    </Button>
                    <Button
                      variant="outline"
                      disabled={busy}
                      onClick={() => {
                        setAsking(undefined)
                      }}
                    >
                      {r.no}
                    </Button>
                  </div>
                </div>
              ) : (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setAsking(rev.id)
                  }}
                >
                  {r.revert}
                </Button>
              ))}
          </li>
        ))}
      </ol>
    </section>
  )
}
