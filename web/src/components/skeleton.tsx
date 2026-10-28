import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// Skeleton stands in for what a page is loading, in its shape: blocks of
// the sizes given (className each), which pulse, asked for less motion
// too, as only their opacity changes. A screen reader hears that it loads.
export function Skeleton({ blocks, className }: { blocks: readonly string[]; className?: string }) {
  const t = useMessages()

  return (
    <div className={cn('space-y-3 lg:space-y-4', className)}>
      <output className="sr-only">{t.loading}</output>
      {blocks.map((block, i) => (
        <div
          // oxlint-disable-next-line react/no-array-index-key -- a fixed list of blocks, which never reorders: its place is its key
          key={i}
          aria-hidden
          className={cn('animate-pulse rounded-lg bg-surface-2', block)}
        />
      ))}
    </div>
  )
}
