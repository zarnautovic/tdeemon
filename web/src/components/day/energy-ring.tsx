import { kcalLeft, kcalStanding } from '@/components/day/left'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// The ring's circle, in the viewBox's units, and its length.
const radius = 56
const length = 2 * Math.PI * radius

// arc is a circle of the ring drawn to share of its length (0 to 1), from
// the top, clockwise, width wide; moving to a new share, it fills to it
// (not with less motion asked for).
function Arc({ share, width, className }: { share: number; width: number; className: string }) {
  return (
    <circle
      cx="66"
      cy="66"
      r={radius}
      fill="none"
      strokeWidth={width}
      strokeLinecap="round"
      strokeDasharray={`${String(share * length)} ${String(length)}`}
      className={`transition-[stroke-dasharray] duration-(--duration-ring) ease-out motion-reduce:transition-none ${className}`}
    />
  )
}

// The middle's number fits within the ring: smaller the more characters
// it has (+1,210 is six), and never under 24 px, large text, as the
// over colour keeps only 3:1.
function sizeOf(n: string): string {
  if (n.length <= 5) {
    return 'text-[2.125rem] leading-[2.375rem]'
  }
  return n.length === 6 ? 'text-[1.75rem] leading-8' : 'text-2xl'
}

// EnergyRing is the day's energy against its target: the ring filled to
// what was eaten, its middle what is left (kcalLeft). Past the target by
// more than the week takes in its stride (kcalStanding), the ring is
// full in the brand's colour, a thinner arc in the over colour, a warm
// neutral and not an error's, runs on by how far over (a whole turn at
// most), and the middle says "+680, over target"; within it, the middle
// says the day is on target. A day that is over (past) says how far it
// stayed under its target, not what is left. Never the colour alone:
// words for a screen reader say it all.
export function EnergyRing({
  eaten,
  target,
  past = false,
}: {
  eaten: number
  target: number
  past?: boolean
}) {
  const t = useMessages()
  const f = useFormat()
  const s = t.day.summary
  const left = kcalLeft(eaten, target)
  const standing = kcalStanding(eaten, target)
  const over = standing === 'over'
  const share = target > 0 ? Math.min(eaten / target, 1) : 0
  const beyond = target > 0 && over ? Math.min(-left / target, 1) : 0

  // The middle's number and its words, and what a screen reader says.
  let shown: string
  let words: string
  let said: string
  if (over) {
    shown = `+${f.number(-left)}`
    words = s.over
    said = s.ringOver(f.number(-left))
  } else if (left < 0 || (past && standing === 'on')) {
    shown = f.signed(-left)
    words = s.on
    said = s.ringOn(f.signed(-left))
  } else if (past) {
    shown = f.number(left)
    words = s.under
    said = s.ringUnder(f.number(left))
  } else {
    shown = f.number(left)
    words = s.left
    said = s.ringLeft(f.number(left))
  }

  return (
    <div className="relative size-34 shrink-0">
      <p className="sr-only">{`${s.ring(f.number(eaten), f.number(target))}, ${said}`}</p>
      <svg aria-hidden viewBox="0 0 132 132" className="size-full -rotate-90">
        <circle
          cx="66"
          cy="66"
          r={radius}
          fill="none"
          strokeWidth="12"
          className="stroke-surface-2"
        />
        {share > 0 && <Arc share={share} width={12} className="stroke-primary" />}
        {beyond > 0 && <Arc share={beyond} width={6} className="stroke-over" />}
      </svg>
      <div
        aria-hidden
        className="absolute inset-4 flex flex-col items-center justify-center text-center"
      >
        <span className={cn('font-semibold tracking-tight', sizeOf(shown), over && 'text-over')}>
          {shown}
        </span>
        <span className="text-caption text-muted-foreground">{words}</span>
      </div>
    </div>
  )
}
