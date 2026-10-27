import type { Nutrients } from '@/api/types'

// MacroSplit is a thin bar of the energy the macros bring, protein's and
// carbs' 4 kcal a gram and fat's 9, each in its colour: for the eye, as
// the grams beside it say the same.
export function MacroSplit({
  nutrients,
}: {
  nutrients: Pick<Nutrients, 'protein' | 'carbs' | 'fat'>
}) {
  const parts = [
    { kcal: nutrients.protein * 4, fill: 'bg-protein' },
    { kcal: nutrients.carbs * 4, fill: 'bg-carbs' },
    { kcal: nutrients.fat * 9, fill: 'bg-fat' },
  ]
  const sum = parts.reduce((s, p) => s + p.kcal, 0)

  return (
    <span
      aria-hidden
      className="flex h-1.5 w-18 shrink-0 overflow-hidden rounded-full bg-surface-2"
    >
      {sum > 0 &&
        parts.map((p) => (
          <span
            key={p.fill}
            className={p.fill}
            style={{ width: `${String((p.kcal / sum) * 100)}%` }}
          />
        ))}
    </span>
  )
}
