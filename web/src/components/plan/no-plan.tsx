import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Sparkles } from 'lucide-react'
import { useId } from 'react'

import { sessionQuery } from '@/api/session'
import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { putOff, usePutOff } from '@/lib/first-steps'
import { cn } from '@/lib/utils'

// NoPlan is the plan's place before there is one: how a plan comes (an
// AI coach proposes it, and the user accepts it here), and the first
// step to it: the profile, while a plan still needs some of it, else an
// AI coach connected, with an access key that may read and log. The
// first steps, put off for later, are brought back to today's page here.
export function NoPlan({ needsProfile }: { needsProfile: boolean }) {
  const t = useMessages()
  const id = useId()
  const e = t.plan.empty
  const user = useQuery(sessionQuery).data?.user.id
  const later = usePutOff(user)

  return (
    <section
      aria-labelledby={id}
      className="space-y-3 rounded-xl border border-dashed bg-card p-5 lg:p-6"
    >
      <span
        aria-hidden
        className="flex size-11 items-center justify-center rounded-full bg-primary-soft text-primary"
      >
        <Sparkles strokeWidth={1.75} />
      </span>
      <h2 id={id} className="text-h2">
        {e.title}
      </h2>
      <p className="text-muted-foreground">{e.text}</p>
      <div className="flex flex-wrap gap-2 pt-1">
        {needsProfile && (
          <Link to="/plan/profile" className={buttonVariants()}>
            {e.profile}
          </Link>
        )}
        <Link
          to="/settings/devices"
          search={{ connect: 'coach' }}
          // cva joins classes without merging: cn lets the outline's border
          // win over the button's transparent one.
          className={cn(buttonVariants({ variant: needsProfile ? 'outline' : 'default' }))}
        >
          {e.coach}
        </Link>
        {later && user !== undefined && (
          <Link
            to="/"
            className={buttonVariants({ variant: 'ghost' })}
            onClick={() => {
              putOff(user, false)
            }}
          >
            {t.firstSteps.show}
          </Link>
        )}
      </div>
    </section>
  )
}
