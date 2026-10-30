import { Info, Scale, TriangleAlert } from 'lucide-react'
import { useId } from 'react'

import { AmountField } from '@/components/day/amount-field'
import { yieldOf } from '@/components/recipes/recipe-values'
import { withPot, yieldOfWeighing, type Weighing } from '@/components/recipes/weighing'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { fieldLabel } from '@/lib/styles'
import { cn } from '@/lib/utils'

// WeightFields are the cooked dish's weight, all optional: its grams
// typed, or worked out from the pot (the full pot less the empty one,
// which is remembered), or "I can't weigh it", when the raw ingredients'
// sum (raw) stands for it. Under the grams, their share of the raw
// ingredients', and a warning when far off it, as the server warns
// (which saves it anyway). last says the weight of the last time.
export function WeightFields({
  value,
  onChange,
  raw,
  remembered,
  last,
  wrong,
}: {
  value: Weighing
  onChange: (w: Weighing) => void
  raw: number | undefined
  // Whether the empty pot is the one remembered.
  remembered: boolean
  last?: string | undefined
  wrong: { weight?: true; pot?: true }
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const grams = (n: number) => t.unit.g(f.number(n))
  const weight = yieldOfWeighing(value, f.parse)
  const share = yieldOf(weight, raw)
  const full = f.parse(value.full)
  const empty = f.parse(value.empty)
  const rawText = raw === undefined ? undefined : grams(raw)
  const set = (change: Partial<Weighing>) => {
    onChange({ ...value, ...change })
  }

  let said
  if (wrong.weight) {
    said = (
      <p className="text-sm text-destructive">
        {t.recipes.weightWrong(f.number(limits.recipe.maxWeight))}
      </p>
    )
  } else if (weight !== undefined && share?.off !== undefined && rawText !== undefined) {
    said = (
      <p className="flex items-start gap-2.5 rounded-md bg-warning/10 px-3 py-2.5 text-small text-warning">
        <TriangleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
        <span>
          {(share.off === 'low' ? t.recipes.yieldLow : t.recipes.yieldHigh)(
            grams(weight),
            f.percent(share.pct, 0),
            rawText,
          )}
        </span>
      </p>
    )
  } else if (share !== undefined && rawText !== undefined) {
    said = (
      <p className="text-small text-muted-foreground">
        {t.recipes.shareOfRaw(f.percent(share.pct, 0), rawText)}
      </p>
    )
  } else {
    said = (
      <p className="text-small text-muted-foreground">
        {rawText === undefined ? t.recipes.noRaw : t.recipes.weighHint(rawText)}
      </p>
    )
  }

  return (
    <section aria-labelledby={`${id}-title`} className="space-y-2">
      <div className="flex items-baseline justify-between gap-2">
        <h2 id={`${id}-title`} className="text-h3">
          {t.recipes.cooked}
        </h2>
        <span className="text-caption text-muted-foreground">{t.recipes.optional}</span>
      </div>
      {!value.cantWeigh && (
        <div className="space-y-2">
          <label htmlFor={`${id}-weight`} className={cn(fieldLabel, 'block')}>
            {t.recipes.cookedWeight}
          </label>
          <div className="flex flex-wrap items-center gap-2">
            <AmountField
              id={`${id}-weight`}
              value={value.weight}
              unit="g"
              aria-invalid={wrong.weight === true}
              aria-describedby={`${id}-said`}
              onChange={(e) => {
                set({ weight: e.currentTarget.value })
              }}
            />
            <Button
              variant="outline"
              aria-expanded={value.pot}
              aria-controls={`${id}-pot`}
              onClick={() => {
                set({ pot: !value.pot })
              }}
            >
              <Scale aria-hidden />
              {t.recipes.fromPot}
            </Button>
          </div>
          {value.pot && (
            <div id={`${id}-pot`} className="space-y-2 rounded-md bg-surface-2 p-3">
              <p className="text-small text-muted-foreground">{t.recipes.potHint}</p>
              <div className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-end gap-2">
                <div className="min-w-0 space-y-1">
                  <label htmlFor={`${id}-full`} className={cn(fieldLabel, 'block')}>
                    {t.recipes.fullPot}
                  </label>
                  <AmountField
                    id={`${id}-full`}
                    value={value.full}
                    unit="g"
                    aria-invalid={wrong.pot === true}
                    onChange={(e) => {
                      onChange(withPot(value, { full: e.currentTarget.value }, f.parse, f.field))
                    }}
                  />
                </div>
                <span aria-hidden className="grid h-11 place-items-center text-muted-foreground">
                  −
                </span>
                <div className="min-w-0 space-y-1">
                  <label htmlFor={`${id}-empty`} className={cn(fieldLabel, 'block truncate')}>
                    {t.recipes.emptyPot}
                    {remembered && ` · ${t.recipes.remembered}`}
                  </label>
                  <AmountField
                    id={`${id}-empty`}
                    value={value.empty}
                    unit="g"
                    aria-invalid={wrong.pot === true}
                    onChange={(e) => {
                      onChange(withPot(value, { empty: e.currentTarget.value }, f.parse, f.field))
                    }}
                  />
                </div>
              </div>
              <p aria-live="polite" className="min-h-5 text-small font-medium">
                {wrong.pot === true ||
                (full !== undefined && empty !== undefined && !(full > empty)) ? (
                  <span className="text-destructive">{t.recipes.potWrong}</span>
                ) : (
                  full !== undefined &&
                  empty !== undefined &&
                  t.recipes.potResult(grams(full - empty))
                )}
              </p>
            </div>
          )}
          <div id={`${id}-said`} aria-live="polite">
            {said}
          </div>
          {last !== undefined && <p className="text-small text-muted-foreground">{last}</p>}
        </div>
      )}
      <div>
        <label className="flex min-h-11 cursor-pointer items-center gap-3">
          <input
            type="checkbox"
            checked={value.cantWeigh}
            aria-describedby={`${id}-cant`}
            className="size-5 shrink-0 accent-primary"
            onChange={(e) => {
              set({ cantWeigh: e.currentTarget.checked })
            }}
          />
          {t.recipes.cantWeigh}
        </label>
        <p id={`${id}-cant`} className="pl-8 text-small text-muted-foreground">
          {t.recipes.cantWeighHint}
        </p>
      </div>
      {value.cantWeigh && (
        <p className="flex items-start gap-2.5 rounded-md bg-surface-2 px-3 py-2.5 text-small">
          <Info aria-hidden className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <span>{rawText === undefined ? t.recipes.noRaw : t.recipes.cantWeighNote(rawText)}</span>
        </p>
      )}
    </section>
  )
}
