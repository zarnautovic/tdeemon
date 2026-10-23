import type { Day } from '@/api/types'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'

const macros = ['protein', 'carbs', 'fat'] as const

// DayTotals is what the day's meals add up to against the target of the
// day's type: the kcal with a bar, what remains or how far over, and each
// macro.
export function DayTotals({ day }: { day: Day }) {
  const t = useMessages()
  const f = useFormat()
  const { total, target, remaining } = day
  const share = target && target.kcal > 0 ? Math.min(total.kcal / target.kcal, 1) : 0

  return (
    <section aria-labelledby="day-eaten" className="space-y-3 rounded-xl border p-4">
      <h2 id="day-eaten" className="text-xs tracking-widest text-muted-foreground uppercase">
        {t.day.eaten}
      </h2>
      <p className="text-3xl font-semibold tabular-nums">
        {f.number(total.kcal)}{' '}
        <span className="text-sm font-normal text-muted-foreground">
          {target ? t.day.ofTarget(f.number(target.kcal)) : 'kcal'}
        </span>
      </p>
      {target && remaining ? (
        <>
          {/* For the eye: the numbers around it say the same. */}
          <div aria-hidden className="h-1.5 overflow-hidden rounded-full bg-muted">
            <div
              className={`h-full rounded-full ${remaining.kcal < 0 ? 'bg-destructive' : 'bg-primary'}`}
              style={{ width: `${String(share * 100)}%` }}
            />
          </div>
          <p className={remaining.kcal < 0 ? 'text-destructive' : 'text-muted-foreground'}>
            {remaining.kcal < 0
              ? t.day.over(f.number(-remaining.kcal))
              : t.day.left(f.number(remaining.kcal))}
          </p>
          <dl className="grid grid-cols-3 gap-2 text-sm">
            {macros.map((m) => (
              <div key={m}>
                <dt className="text-muted-foreground">{t.day[m]}</dt>
                <dd className="tabular-nums">
                  {f.number(total[m])} / {f.number(target[m])} g
                </dd>
              </div>
            ))}
          </dl>
        </>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t.day.noTarget}{' '}
          {macros.map((m) => `${t.day.short[m]} ${f.number(total[m])}`).join(' · ')}
        </p>
      )}
    </section>
  )
}
