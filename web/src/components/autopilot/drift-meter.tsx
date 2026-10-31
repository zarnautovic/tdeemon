import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// DriftMeter is how far the autopilot moved the target since the last
// one the user agreed to, against its limit: its net kcal, signed, of the
// limit, as a bar; in the warning's colour once the next step (step)
// would pass it, and then said in words, as the next step is proposed
// instead. Before the limit has a base, at the first step, the kcal
// alone, and that the limit comes then: never a number that moves.
export function DriftMeter({
  used,
  limit,
  step = 0,
  compact = false,
}: {
  used: number
  limit: number | undefined
  step?: number
  compact?: boolean
}) {
  const t = useMessages()
  const f = useFormat()
  const a = t.autopilot
  const value = compact ? 'text-h3' : 'font-semibold'
  if (limit === undefined) {
    return (
      <div className="space-y-1">
        <p className={value}>{a.driftUsed(f.signed(used))}</p>
        <p className="text-caption text-muted-foreground">{a.driftNoBase}</p>
      </div>
    )
  }
  const moved = Math.abs(used)
  const full = step > 0 && moved + step > limit
  const said = a.driftValue(f.signed(used), f.number(limit))

  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-baseline justify-between gap-x-2">
        <p className={value}>{said}</p>
        {!compact && (
          <p className="text-small text-muted-foreground">
            {a.driftLeft(f.number(Math.max(0, limit - moved)))}
          </p>
        )}
      </div>
      <progress
        aria-label={a.drift}
        aria-valuetext={said}
        max={limit}
        value={Math.min(moved, limit)}
        className={cn(
          'block h-2 w-full appearance-none overflow-hidden rounded-full bg-surface-2 [&::-moz-progress-bar]:rounded-full [&::-webkit-progress-bar]:bg-surface-2 [&::-webkit-progress-value]:rounded-full',
          compact && 'bg-card [&::-webkit-progress-bar]:bg-card',
          full
            ? '[&::-moz-progress-bar]:bg-warning [&::-webkit-progress-value]:bg-warning'
            : '[&::-moz-progress-bar]:bg-primary [&::-webkit-progress-value]:bg-primary',
        )}
      />
      {full && <p className="text-caption text-warning">{a.driftFull}</p>}
    </div>
  )
}
