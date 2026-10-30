import { Link } from '@tanstack/react-router'
import { ChevronRight, type LucideIcon } from 'lucide-react'
import { useId, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

// The pages a row leads to: those of the plan's list and the settings'.
export type RowPath =
  | '/plan/profile'
  | '/plan/limits'
  | '/plan/notes'
  | '/settings/appearance'
  | '/settings/account'
  | '/settings/devices'
  | '/settings/notifications'

const rowClass =
  'flex min-h-14 w-full items-center gap-3 px-4 py-2.5 text-left outline-none hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset lg:px-5'

// ListRow is a row of a list of settings, as a whole a link to its page
// (to) or a button (onClick): an icon, a name, the summary of its value
// under it, cut to two lines, and, for a link, a chevron. Its name is its
// title, from its words (the summary hidden from them), and the summary
// describes it: so the name holds what it shows, as one is labelled. It is 56 px high at least, a
// thumb's target with room. A summary that warns (a profile a plan still
// needs) says so in its words, its colour aside.
export function ListRow({
  icon: Icon,
  title,
  summary,
  warn = false,
  ...action
}: {
  icon: LucideIcon
  title: string
  summary?: ReactNode
  warn?: boolean
} & (
  | { to: RowPath; onClick?: never; disabled?: never }
  | { to?: never; onClick: () => void; disabled?: boolean }
)) {
  const id = useId()
  const named = {
    'aria-describedby': summary === undefined ? undefined : `${id}-summary`,
  }
  const content = (
    <>
      <span
        aria-hidden
        className="flex size-9 shrink-0 items-center justify-center rounded-md bg-surface-2 text-muted-foreground"
      >
        <Icon strokeWidth={1.75} className="size-4.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block font-medium">{title}</span>
        {summary !== undefined && (
          <span
            id={`${id}-summary`}
            aria-hidden
            className={cn(
              'line-clamp-2 block text-small',
              warn ? 'text-warning' : 'text-muted-foreground',
            )}
          >
            {summary}
          </span>
        )}
      </span>
    </>
  )

  return (
    <li>
      {action.to === undefined ? (
        <button
          type="button"
          {...named}
          disabled={action.disabled}
          onClick={action.onClick}
          className={cn(rowClass, 'disabled:opacity-45')}
        >
          {content}
        </button>
      ) : (
        <Link to={action.to} {...named} className={rowClass}>
          {content}
          <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
        </Link>
      )}
    </li>
  )
}
