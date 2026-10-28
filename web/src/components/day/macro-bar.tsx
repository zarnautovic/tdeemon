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
// eaten of the target's and what is left (or how far over, gramsLeft), and
// a bar filled to them, which fills to a new value (not with less motion
// asked for). The words say what the bar shows.
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
  const left = gramsLeft(eaten, target)
  const share = target > 0 ? Math.min(eaten / target, 1) : 0

  return (
    <div className="space-y-1.5">
      <p className="flex flex-wrap justify-between gap-x-2 text-small">
        <span className="font-semibold">{t.day[macro]}</span>
        <span className="text-muted-foreground">
          <b className="font-semibold text-foreground">{f.grams(eaten)}</b>{' '}
          {t.day.summary.ofTarget(f.grams(target))} ·{' '}
          <span className={left < 0 ? 'text-destructive' : ''}>
            {left < 0 ? t.day.summary.overBy(f.grams(-left)) : t.day.summary.toGo(f.grams(left))}
          </span>
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
