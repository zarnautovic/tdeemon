import { cn } from '@/lib/utils'

// BrandMark is tdeemon's mark: a trend's curve on the brand's colour. It
// is drawn beside the name, which says it, so it says nothing itself.
export function BrandMark({ className }: { className?: string }) {
  return (
    <svg aria-hidden viewBox="0 0 32 32" className={cn('size-7 shrink-0', className)}>
      <rect width="32" height="32" rx="9" className="fill-primary" />
      <path
        d="M8 20c3 0 4-8 8-8s5 6 8 6"
        fill="none"
        strokeWidth="2.5"
        strokeLinecap="round"
        className="stroke-primary-foreground"
      />
    </svg>
  )
}
