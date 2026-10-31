import { useReviewWords } from '@/components/autopilot/review-words'
import { useRationale } from '@/components/plan/change-words'
import type { Proposal } from '@/components/plan/proposal-changes'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// Rationale is why a change of the plan was made: a coach's words, the
// app's code in the language, or, for the autopilot's (the code
// `autopilot`), its weekly review said from its numbers: what it
// decided, then why; brief, the first reason alone, as the plan's history
// says it above the changes. One it proposed past its drift limit, while
// it waits, says so, and what an answer does.
export function Rationale({ proposal, brief = false }: { proposal: Proposal; brief?: boolean }) {
  const t = useMessages()
  const f = useFormat()
  const rationale = useRationale()
  const say = useReviewWords()
  const c = proposal.check_in
  if (proposal.rationale !== 'autopilot' || c === undefined) {
    return <p className="whitespace-pre-line">{rationale(proposal.rationale)}</p>
  }
  const words = say(c)
  const a = t.autopilot

  return (
    <div className="space-y-1">
      <p className="font-medium">{words.headline}</p>
      {(brief ? words.why.slice(0, 1) : words.why).map((w) => (
        <p key={w} className="text-sm text-muted-foreground">
          {w}
        </p>
      ))}
      {c.past_drift && proposal.status === 'pending' && (
        <div className="mt-2 space-y-0.5 rounded-md border border-warning/40 bg-warning/10 p-3 text-sm">
          <p className="font-semibold text-warning">{a.pastDriftTitle}</p>
          <p>
            {c.drift_limit_kcal === undefined
              ? a.pastDriftNoLimit(f.signed(c.drift_kcal))
              : a.pastDrift(f.signed(c.drift_kcal), f.number(c.drift_limit_kcal))}
          </p>
        </div>
      )}
    </div>
  )
}
