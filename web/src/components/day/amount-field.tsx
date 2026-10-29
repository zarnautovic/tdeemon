import { ChevronDown } from 'lucide-react'
import type { ComponentProps } from 'react'

// AmountField is an item's amount and its unit as one field, narrow
// enough to leave a phone's row its item's name: the number typed, and
// after it the unit, in words (unit), or a choice of the units its food
// is given in (select, its options as its children), which a phone shows
// in its own picker. Its frame is drawn as an outline, so the number and
// the unit each are as high as the field, a finger's 44 px; the frame
// shows the focus in it (the unit's, its own background too) and the
// number invalid. Hidden, the field and its unit are still sent with
// their form.
export function AmountField({
  unit,
  select,
  hidden,
  ...input
}: Omit<ComponentProps<'input'>, 'className'> & {
  unit?: string | undefined
  select?: ComponentProps<'select'> | undefined
}) {
  return (
    <span
      hidden={hidden}
      className="flex h-11 w-30 shrink-0 items-center rounded-md bg-card outline -outline-offset-1 outline-input focus-within:ring-2 focus-within:ring-ring/30 focus-within:outline-ring has-[input[aria-invalid=true]]:outline-destructive"
    >
      <input
        inputMode="decimal"
        autoComplete="off"
        className="h-full w-full min-w-0 flex-1 rounded-l-md bg-transparent pl-3 text-base tabular-nums outline-none disabled:opacity-45"
        {...input}
      />
      {select ? (
        <span className="relative flex h-full shrink-0 items-center">
          <select
            {...select}
            className="field-sizing-content h-full max-w-20 min-w-11 cursor-pointer appearance-none truncate rounded-r-md bg-transparent pr-5 pl-1 text-small text-muted-foreground outline-none focus-visible:bg-muted focus-visible:text-foreground disabled:opacity-45"
          />
          <ChevronDown
            aria-hidden
            className="pointer-events-none absolute right-1 size-3.5 text-muted-foreground"
          />
        </span>
      ) : (
        unit && (
          <span className="max-w-14 shrink-0 truncate pr-3 pl-1 text-small text-muted-foreground">
            {unit}
          </span>
        )
      )}
    </span>
  )
}
