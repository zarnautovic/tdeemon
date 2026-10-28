import { useId, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

// ListGroup is a list of rows (ListRow) in one card, a line between two
// rows, under its heading, if it has one (a page's title may name it
// instead); what follows the card (foot) is under it.
export function ListGroup({
  title,
  className,
  foot,
  children,
}: {
  title?: string | undefined
  className?: string | undefined
  foot?: ReactNode
  children: ReactNode
}) {
  const id = useId()

  return (
    <section
      aria-labelledby={title === undefined ? undefined : id}
      className={cn('space-y-2', className)}
    >
      {title !== undefined && (
        <h2 id={id} className="px-1 text-small text-muted-foreground">
          {title}
        </h2>
      )}
      <ul className="divide-y overflow-hidden rounded-lg border bg-card shadow-card">{children}</ul>
      {foot}
    </section>
  )
}
