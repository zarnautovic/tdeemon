import { useId, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

// SegmentedControl picks one of a few options, as segments of one
// control: radios in a fieldset whose legend names it, so a screen reader
// says them as one group and the arrow keys move between them. A segment
// is 44 px high on a phone, as a button. children follow the segments
// in the fieldset (a hint, which describedBy names).
export function SegmentedControl<T extends string>({
  legend,
  options,
  value,
  onChange,
  hideLegend = false,
  describedBy,
  className,
  children,
}: {
  legend: ReactNode
  options: readonly { value: T; label: ReactNode }[]
  value: T
  onChange: (value: T) => void
  // The legend for a screen reader only, where a heading names the
  // control already.
  hideLegend?: boolean
  describedBy?: string
  className?: string
  children?: ReactNode
}) {
  const id = useId()

  return (
    <fieldset className={cn('space-y-2', className)} aria-describedby={describedBy}>
      <legend className={hideLegend ? 'sr-only' : 'font-medium'}>{legend}</legend>
      <div className="flex gap-1 rounded-md bg-surface-2 p-1">
        {options.map((option) => (
          <label
            key={option.value}
            className="flex h-11 min-w-0 flex-1 cursor-pointer items-center justify-center rounded-sm px-2 text-sm text-muted-foreground transition-colors has-checked:bg-card has-checked:font-semibold has-checked:text-foreground has-checked:shadow-card has-focus-visible:ring-2 has-focus-visible:ring-ring motion-reduce:transition-none lg:h-9"
          >
            <input
              type="radio"
              name={`${id}-choice`}
              className="sr-only"
              checked={value === option.value}
              onChange={() => {
                onChange(option.value)
              }}
            />
            <span className="truncate">{option.label}</span>
          </label>
        ))}
      </div>
      {children}
    </fieldset>
  )
}
