import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

// tones are a badge's colours: plain, a warning's (an estimate) and the
// brand's (the user's own, a scope).
const tones = {
  neutral: 'bg-surface-2 text-muted-foreground',
  warning: 'bg-warning/10 text-warning',
  primary: 'bg-primary-soft text-primary',
} as const

// Badge is a short mark beside a thing: AI, an estimate, a token's
// scope. Its words say what it marks; its tone only colours it.
export function Badge({
  tone = 'neutral',
  className,
  ...props
}: ComponentProps<'span'> & { tone?: keyof typeof tones }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-caption whitespace-nowrap',
        tones[tone],
        className,
      )}
      {...props}
    />
  )
}
