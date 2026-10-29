import type { ComponentProps } from 'react'

import { useFormat } from '@/i18n/format'
import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// TimeInput is a field for a time of day on the language's clock, as the
// rest of the app shows times (useFormat's clock): 7:30 PM in English,
// 19:30 in Croatian, either typed (parseClock reads both, and 1930) and
// the same in every browser, where a browser's own field may show
// another clock, in its own language, cut short.
export function TimeInput({ className, ...props }: ComponentProps<'input'>) {
  const f = useFormat()
  return (
    <input
      inputMode={f.hour12 ? 'text' : 'decimal'}
      autoComplete="off"
      // 12:30 p.m. on a 12-hour clock, 19:30 on a 24-hour one.
      maxLength={f.hour12 ? 10 : 5}
      placeholder={f.clock('19:30')}
      className={cn(inputClass, 'tabular-nums', className)}
      {...props}
    />
  )
}
