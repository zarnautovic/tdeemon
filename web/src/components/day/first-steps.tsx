import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Circle, CircleCheck } from 'lucide-react'
import { useId, type ReactNode } from 'react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import type { Day } from '@/api/types'
import { CopyField } from '@/components/copy-field'
import { WeighIn } from '@/components/day/weigh-in'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { putOff, usePutOff } from '@/lib/first-steps'
import { linkTarget } from '@/lib/styles'
import { cn } from '@/lib/utils'

const link = cn(
  linkTarget,
  'font-semibold text-primary underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring',
)

// FirstSteps are a new user's way to a plan, in place of today's hero
// (children) while there is no plan: a card of the steps, ticked as each
// is done, and how many are: the profile, a first weigh-in (in the card),
// an AI coach or a phone connected (it called with its access key), and
// a first meal. Once the profile, a weigh-in and the connection are
// there, what to ask the AI coach for a plan, to copy. Put off for later,
// it goes until the plan's page brings it back; a proposal that waits, or
// a plan, ends it.
export function FirstSteps({ day, children }: { day: Day; children: ReactNode }) {
  const me = $api.useQuery('get', '/api/v1/me')
  const plan = $api.useQuery('get', '/api/v1/plan')
  const user = useQuery(sessionQuery).data?.user.id
  const later = usePutOff(user)
  const before =
    plan.data?.phases.length === 0 &&
    plan.data.targets.length === 0 &&
    plan.data.pending === undefined
  if (
    me.data?.today !== day.date ||
    day.target !== undefined ||
    !before ||
    user === undefined ||
    later
  ) {
    return children
  }
  return <Steps day={day} user={user} />
}

// Steps is the card of the first steps.
function Steps({ day, user }: { day: Day; user: string }) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const s = t.firstSteps
  const me = $api.useQuery('get', '/api/v1/me')
  const summary = $api.useQuery('get', '/api/v1/summary')
  const tokens = $api.useQuery('get', '/api/v1/tokens')
  const meals = $api.useQuery('get', '/api/v1/meals/recent')
  const weight = day.metrics.find((m) => m.kind === 'weight')
  const done = {
    profile: me.data?.profile.missing.length === 0,
    weighIn: weight !== undefined || summary.data?.trend !== undefined,
    connect: tokens.data?.tokens.some((k) => k.last_used_at !== undefined) === true,
    meal: day.meals.length > 0 || (meals.data?.meals.length ?? 0) > 0,
  }
  const count = Object.values(done).filter(Boolean).length

  return (
    <section aria-labelledby={id} className="space-y-3 rounded-lg border bg-card p-4">
      <div className="flex items-baseline justify-between gap-2">
        <h2 id={id} className="text-h3">
          {s.title}
        </h2>
        <span className="text-small text-muted-foreground">{s.count(count, 4)}</span>
      </div>
      <ol className="space-y-1">
        <Step done={done.profile}>
          {done.profile ? (
            s.profileDone
          ) : (
            <Link to="/plan/profile" className={link}>
              {s.profile}
            </Link>
          )}
        </Step>
        <Step done={done.weighIn}>
          {done.weighIn ? (
            [s.weighIn, ...(weight === undefined ? [] : [`${f.number(weight.value, 1)} kg`])].join(
              ' · ',
            )
          ) : (
            <WeighIn date={day.date} label={s.weighIn} />
          )}
        </Step>
        <Step done={done.connect}>
          {done.connect ? (
            s.connectDone
          ) : (
            <Link to="/settings/devices" search={{ connect: 'choose' }} className={link}>
              {s.connect}
            </Link>
          )}
        </Step>
        <Step done={done.meal}>
          {done.meal ? (
            s.mealDone
          ) : (
            <Link to="/" search={{ log: true }} className={link}>
              {s.meal}
            </Link>
          )}
        </Step>
      </ol>
      {done.profile && done.weighIn && done.connect && (
        <div className="space-y-2 rounded-md bg-primary-soft p-3">
          <p className="font-semibold">{s.ask}</p>
          <CopyField label={s.write} value={t.connect.message} />
        </div>
      )}
      <Button
        variant="ghost"
        className="-ml-3"
        onClick={() => {
          putOff(user, true)
        }}
      >
        {s.later}
      </Button>
    </section>
  )
}

// Step is a first step, ticked once done, which a screen reader hears
// before its words.
function Step({ done, children }: { done: boolean; children: ReactNode }) {
  const t = useMessages()
  const Icon = done ? CircleCheck : Circle

  return (
    <li className="flex min-h-11 items-center gap-3">
      <Icon
        aria-hidden
        strokeWidth={1.75}
        className={cn('size-5 shrink-0', done ? 'text-success' : 'text-muted-foreground')}
      />
      <span className="sr-only">{done ? t.firstSteps.done : t.firstSteps.toDo}</span>
      <div className="min-w-0 flex-1">{children}</div>
    </li>
  )
}
