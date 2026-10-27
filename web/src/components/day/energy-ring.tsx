import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

// The ring's circle, in the viewBox's units, and its length.
const radius = 56
const length = 2 * Math.PI * radius

// arc is a circle of the ring drawn to share of its length (0 to 1), from
// the top, clockwise; moving to a new share, it fills to it (not with
// less motion asked for).
function Arc({ share, className }: { share: number; className: string }) {
  return (
    <circle
      cx="66"
      cy="66"
      r={radius}
      fill="none"
      strokeWidth="12"
      strokeLinecap="round"
      strokeDasharray={`${String(share * length)} ${String(length)}`}
      className={`transition-[stroke-dasharray] duration-(--duration-ring) ease-out motion-reduce:transition-none ${className}`}
    />
  )
}

// EnergyRing is the day's energy against its target: the ring filled to
// what was eaten, its middle what is left. Past the target the ring is
// full, a second arc in the destructive colour runs on by how far over
// (a whole turn at most), and the middle says how far over: never the
// colour alone. Words for a screen reader say it all.
export function EnergyRing({ eaten, target }: { eaten: number; target: number }) {
  const t = useMessages()
  const f = useFormat()
  const over = eaten - target
  const share = target > 0 ? Math.min(eaten / target, 1) : 0
  const beyond = target > 0 && over > 0 ? Math.min(over / target, 1) : 0
  const said =
    over > 0
      ? t.day.summary.ringOver(f.number(over))
      : t.day.summary.ringLeft(f.number(target - eaten))

  return (
    <div className="relative size-34 shrink-0">
      <p className="sr-only">{`${t.day.summary.ring(f.number(eaten), f.number(target))}, ${said}`}</p>
      <svg aria-hidden viewBox="0 0 132 132" className="size-full -rotate-90">
        <circle
          cx="66"
          cy="66"
          r={radius}
          fill="none"
          strokeWidth="12"
          className="stroke-surface-2"
        />
        {share > 0 && <Arc share={share} className="stroke-primary" />}
        {beyond > 0 && <Arc share={beyond} className="stroke-destructive" />}
      </svg>
      <div aria-hidden className="absolute inset-0 flex flex-col items-center justify-center">
        <span
          className={`text-[2.125rem] leading-[2.375rem] font-semibold tracking-tight ${over > 0 ? 'text-destructive' : ''}`}
        >
          {over > 0 ? `+${f.number(over)}` : f.number(target - eaten)}
        </span>
        <span className="text-caption text-muted-foreground">
          {over > 0 ? t.day.summary.over : t.day.summary.left}
        </span>
      </div>
    </div>
  )
}
