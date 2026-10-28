import { cn } from '@/lib/utils'

// Skeleton is a block in the place and shape of something that comes,
// pulsing while it does (still, with less motion asked for). A screen
// reader skips it: the part it stands in says it is loading.
export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      aria-hidden
      className={cn('rounded-md bg-surface-2 motion-safe:animate-pulse', className)}
    />
  )
}
