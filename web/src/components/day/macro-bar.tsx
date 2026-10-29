import { gramsLeft } from '@/components/day/left'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

export type Macro = 'protein' | 'carbs' | 'fat'

// Each macro's colour, which goes with its name and numbers, never alone.
const fills: Record<Macro, string> = {
  protein: 'bg-protein',
  carbs: 'bg-carbs',
  fat: 'bg-fat',
}

// MacroBar is a macro of the day against its target: its name, the grams
// eaten of the target's and what is left, and a bar filled to them, which
// fills to a new value (not with less motion asked for). Past the target
// nothing is an error: protein's target is a floor, which the day has
// reached, said in the brand's colour with how far past it; carbs and fat
// say how far past theirs, plainly. The words say what the bar shows.
export function MacroBar({
  macro,
  eaten,
  target,
}: {
  macro: Macro
  eaten: number
  target: number
}) {
  const t = useMessages()
  const f = useFormat()
  const s = t.day.summary
  const left = gramsLeft(eaten, target)
  const share = target > 0 ? Math.min(eaten / target, 1) : 0

  let after = <>{s.toGo(f.grams(left))}</>
  if (macro === 'protein' && left <= 0 && target > 0) {
    after = (
      <span className="font-semibold text-primary">
        <span aria-hidden>✓ </span>
        {left < 0 ? s.reachedBy(f.grams(-left)) : s.reached}
      </span>
    )
  } else if (left < 0) {
    after = <>{s.overBy(f.grams(-left))}</>
  }

  return (
    <div className="space-y-1.5">
      <p className="flex flex-wrap justify-between gap-x-2 text-small">
        <span className="font-semibold">{t.day[macro]}</span>
        <span className="text-muted-foreground">
          <b className="font-semibold text-foreground">{f.grams(eaten)}</b>{' '}
          {s.ofTarget(f.grams(target))} · {after}
        </span>
      </p>
      <div aria-hidden className="h-2 rounded-full bg-surface-2">
        <div
          className={`h-full rounded-full transition-[width] duration-(--duration-ring) ease-out motion-reduce:transition-none ${fills[macro]}`}
          style={{ width: `${String(share * 100)}%` }}
        />
      </div>
    </div>
  )
}
