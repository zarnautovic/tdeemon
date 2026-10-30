import { Minus, Plus } from 'lucide-react'
import { useId } from 'react'

import { portionsOf } from '@/components/recipes/recipe-values'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// PortionsField is how many portions a dish makes, optional: typed, or a
// step up or down, with the grams of one (≈ when the dish was not
// weighed). weight is the dish's grams, when known.
export function PortionsField({
  value,
  onChange,
  weight,
  weighed,
  wrong,
}: {
  value: string
  onChange: (text: string) => void
  weight: number | undefined
  weighed: boolean
  wrong: boolean
}) {
  const t = useMessages()
  const f = useFormat()
  const id = useId()
  const n = portionsOf(value, f.parse)
  const most = limits.recipe.maxPortions
  const step = (by: number) => {
    onChange(String(Math.min(most, Math.max(1, (n ?? 0) + by))))
  }

  return (
    <section className="space-y-2">
      <div className="flex items-baseline justify-between gap-2">
        <label htmlFor={`${id}-portions`} className="text-h3">
          {t.recipes.portionsTitle}
        </label>
        <span className="text-caption text-muted-foreground">{t.recipes.optional}</span>
      </div>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="icon"
          aria-label={t.recipes.fewer}
          disabled={n === undefined || n <= 1}
          onClick={() => {
            step(-1)
          }}
        >
          <Minus aria-hidden />
        </Button>
        <input
          id={`${id}-portions`}
          value={value}
          inputMode="numeric"
          autoComplete="off"
          aria-invalid={wrong}
          aria-describedby={`${id}-hint`}
          className={cn(inputClass, 'w-18 text-center tabular-nums')}
          onChange={(e) => {
            onChange(e.currentTarget.value)
          }}
        />
        <Button
          variant="outline"
          size="icon"
          aria-label={t.recipes.more}
          disabled={n !== undefined && n >= most}
          onClick={() => {
            step(1)
          }}
        >
          <Plus aria-hidden />
        </Button>
        <span aria-live="polite" className="min-w-0 flex-1 text-small text-muted-foreground">
          {n !== undefined && weight !== undefined
            ? t.recipes.each(`${weighed ? '' : '≈ '}${t.unit.g(f.number(weight / n))}`)
            : value.trim() === '' && t.recipes.undivided}
        </span>
      </div>
      <p
        id={`${id}-hint`}
        className={wrong ? 'text-sm text-destructive' : 'text-small text-muted-foreground'}
      >
        {wrong ? t.recipes.portionsWrong(f.number(most)) : t.recipes.portionsHint}
      </p>
    </section>
  )
}
