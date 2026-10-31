import { Link } from '@tanstack/react-router'
import { ChevronRight, Gauge } from 'lucide-react'
import { useId } from 'react'

import type { components } from '@/api/schema'
import { DriftMeter } from '@/components/autopilot/drift-meter'
import { useReviewWords, type Review, type ReviewWords } from '@/components/autopilot/review-words'
import { weekdayOf } from '@/components/autopilot/weekday'
import { useDate } from '@/components/plan/proposal-text'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { linkTarget } from '@/lib/styles'
import { cn } from '@/lib/utils'

type Plan = components['schemas']['PlanBody']
type Autopilot = components['schemas']['AutopilotBody']

// The tone of a review's decision: a step's the brand's, a proposal's the
// warning's (it waits for the user), a hold the success's, no step plain.
// The words say which; the colour only marks it.
const tones = {
  applied: 'bg-primary',
  proposed: 'bg-warning',
  hold: 'bg-success',
  skipped: 'bg-muted-foreground',
} as const

// AutopilotCard is the autopilot on the plan: whether it applies its
// steps or asks first, its last weekly review (what it decided, and why
// in the review's numbers, the rest of them a tap away), when the next
// comes, and in autopilot how far it moved the target from the one the
// user agreed to, against the limit, or else the step it would propose;
// with its settings a tap away. A step it applied is taken back in the
// plan's history, as any change of the plan. Its own proposal that waits
// is said whole in its card above: here, only that it waits. Without the
// autopilot (autopilot), which did not come, what the plan says of it.
export function AutopilotCard({
  plan,
  autopilot: ap,
}: {
  plan: Plan
  autopilot: Autopilot | undefined
}) {
  const t = useMessages()
  const f = useFormat()
  const date = useDate()
  const say = useReviewWords()
  const id = useId()
  const a = t.autopilot
  const mode = ap?.mode ?? plan.autopilot.mode
  const next = ap?.next ?? plan.autopilot.next
  const on = mode === 'autopilot'
  const last = ap?.last
  const waiting =
    last?.outcome === 'proposed' &&
    last.proposal_id !== undefined &&
    plan.pending?.id === last.proposal_id
  // The first step, when the next review cannot take it yet: the server
  // says when, after its waits and calibration's.
  const first = ap?.first_step !== undefined && ap.first_step > next ? ap.first_step : undefined

  return (
    <section aria-labelledby={id} className="space-y-3 rounded-lg border bg-card p-4 lg:p-5">
      <header className="flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span
            aria-hidden
            className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary-soft text-primary"
          >
            <Gauge strokeWidth={1.75} className="size-4.5" />
          </span>
          <h2 id={id} className="text-h3">
            {a.name}
          </h2>
          <Badge tone={on ? 'primary' : 'neutral'}>{on ? a.on : a.asks}</Badge>
        </div>
        <Link
          to="/plan/autopilot"
          aria-label={a.settingsLabel}
          className={buttonVariants({ variant: 'ghost', size: 'sm', className: '-mr-2' })}
        >
          {a.settings}
          <ChevronRight aria-hidden />
        </Link>
      </header>
      {last !== undefined && <LastReview review={last} waiting={waiting} words={say(last)} />}
      {first !== undefined && (
        <p className="text-sm text-muted-foreground">{a.firstStep(date(first))}</p>
      )}
      <dl className="grid grid-cols-2 gap-3">
        <div className="space-y-0.5 rounded-md bg-surface-2 p-3">
          <dt className="text-small text-muted-foreground">{a.next}</dt>
          <dd className="text-h3">{a.nextOn(f.weekday(next), date(next))}</dd>
          <dd className="text-caption text-muted-foreground">{a.every[weekdayOf(next)]}</dd>
        </div>
        <div className="space-y-1 rounded-md bg-surface-2 p-3">
          <dt className="text-small text-muted-foreground">{on ? a.drift : a.step}</dt>
          {on ? (
            <dd>
              <DriftMeter
                used={ap?.drift.used_kcal ?? plan.autopilot.drift_kcal}
                limit={ap === undefined ? plan.autopilot.drift_limit_kcal : ap.drift.limit_kcal}
                step={ap?.step_kcal ?? 0}
                compact
              />
            </dd>
          ) : (
            <Step kcal={ap?.step_kcal} />
          )}
        </div>
      </dl>
    </section>
  )
}

// Step is the step a review would propose: up to the limits' a day.
function Step({ kcal }: { kcal: number | undefined }) {
  const t = useMessages()
  const f = useFormat()
  const a = t.autopilot
  if (kcal === undefined) {
    return <dd className="text-h3">–</dd>
  }
  if (kcal === 0) {
    return <dd className="text-sm">{a.stepNone}</dd>
  }
  return (
    <>
      <dd className="text-h3">{a.stepUpTo(f.number(kcal))}</dd>
      <dd className="text-caption text-muted-foreground">{a.stepIfAccepted}</dd>
    </>
  )
}

// LastReview is the last weekly review: when, what it decided, and why at
// a glance, the rest of its numbers a tap away; the user's answer to its
// proposal, and, of a step, where it is taken back.
function LastReview({
  review,
  waiting,
  words,
}: {
  review: Review
  waiting: boolean
  words: ReviewWords
}) {
  const t = useMessages()
  const date = useDate()
  const a = t.autopilot
  const [first, ...rest] = words.why

  return (
    <div className="space-y-1.5">
      <p className="text-small text-muted-foreground">{a.lastOn(date(review.date))}</p>
      <p className="flex items-start gap-2 font-semibold">
        <span
          aria-hidden
          className={cn('mt-2 size-2 shrink-0 rounded-full', tones[review.outcome])}
        />
        <span>{words.headline}</span>
      </p>
      {waiting ? (
        <p className="text-sm text-muted-foreground">{a.waitsAbove}</p>
      ) : (
        <>
          {first !== undefined && <p className="text-sm text-muted-foreground">{first}</p>}
          {rest.length > 0 && (
            <details className="group text-sm text-muted-foreground">
              <summary className="flex min-h-11 cursor-pointer list-none items-center gap-1 font-medium text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                <ChevronRight
                  aria-hidden
                  className="size-4 transition-transform group-open:rotate-90 motion-reduce:transition-none"
                />
                {a.numbers}
              </summary>
              <div className="space-y-1.5 pb-1">
                {rest.map((w) => (
                  <p key={w}>{w}</p>
                ))}
              </div>
            </details>
          )}
        </>
      )}
      {review.answer !== undefined && (
        <p className="text-sm font-medium">
          {review.answer === 'accepted' ? a.accepted : a.rejected}
        </p>
      )}
      {review.outcome === 'applied' && (
        <p className="text-sm">
          <a
            href="#plan-history"
            className={cn(
              linkTarget,
              'font-medium text-primary underline-offset-2 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring',
            )}
          >
            {a.takeBack}
          </a>
        </p>
      )}
    </div>
  )
}
