import type { Food } from '@/api/types'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// EnergySplit is where a food's energy comes from: a bar of the energy
// the macros bring, protein's and carbs' 4 kcal a gram and fat's 9, each
// in its colour, and each macro's grams and share in words.
export function EnergySplit({
  nutrients,
}: {
  nutrients: Pick<Food['nutrients'], 'protein' | 'carbs' | 'fat'>
}) {
  const t = useMessages()
  const f = useFormat()
  const parts = [
    { macro: 'protein', grams: nutrients.protein, kcal: nutrients.protein * 4, fill: 'bg-protein' },
    { macro: 'carbs', grams: nutrients.carbs, kcal: nutrients.carbs * 4, fill: 'bg-carbs' },
    { macro: 'fat', grams: nutrients.fat, kcal: nutrients.fat * 9, fill: 'bg-fat' },
  ] as const
  const sum = parts.reduce((s, p) => s + p.kcal, 0)

  return (
    <div className="space-y-3">
      <div aria-hidden className="flex h-2.5 gap-0.5 overflow-hidden rounded-full bg-surface-2">
        {sum > 0 &&
          parts.map((p) => (
            <span
              key={p.macro}
              className={p.fill}
              style={{ width: `${String((p.kcal / sum) * 100)}%` }}
            />
          ))}
      </div>
      <dl className="grid grid-cols-3 gap-2">
        {parts.map((p) => (
          <div key={p.macro} className="min-w-0">
            <dt className="flex items-center gap-1.5 text-caption text-muted-foreground">
              <span aria-hidden className={cn('size-2 shrink-0 rounded-full', p.fill)} />
              {t.day[p.macro]}
            </dt>
            <dd className="text-small font-semibold">
              {t.foods.share(f.grams(p.grams), f.percent(sum > 0 ? (p.kcal / sum) * 100 : 0, 0))}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
