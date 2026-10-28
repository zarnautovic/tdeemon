import type { ComponentProps } from 'react'

import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// TimeInput is a field for a time of day on a 24-hour clock, typed as
// 19:30 (parseClock reads it): the same in every language and browser,
// where a browser's own field may show 12 hours, in its own language,
// cut short.
export function TimeInput({ className, ...props }: ComponentProps<'input'>) {
  return (
    <input
      inputMode="decimal"
      autoComplete="off"
      maxLength={5}
      placeholder="19:30"
      className={cn(inputClass, 'tabular-nums', className)}
      {...props}
    />
  )
}
