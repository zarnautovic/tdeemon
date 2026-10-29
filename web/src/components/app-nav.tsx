import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  CalendarDays,
  ChartLine,
  Plus,
  Settings,
  Target,
  Utensils,
  type LucideIcon,
} from 'lucide-react'
import { useId, type ReactNode } from 'react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import { ThemeToggle } from '@/components/theme-toggle'
import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// A section's link: on a phone a tab of the bar, its icon over its name;
// on a desktop a row of the sidebar. TanStack Router's Link marks the
// current one aria-current="page".
const sectionClass =
  'group flex min-h-12 flex-col items-center justify-center gap-0.5 rounded-md text-caption text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:font-semibold aria-[current=page]:text-primary lg:h-10 lg:min-h-0 lg:flex-row lg:justify-start lg:gap-2.5 lg:px-3 lg:text-sm lg:font-medium lg:text-foreground lg:hover:bg-muted lg:aria-[current=page]:bg-primary-soft lg:aria-[current=page]:text-primary'

// A section's dot says something waits there: on the icon in the bar,
// after the name in the sidebar; to a screen reader, mark describes the
// link, whose name stays the section's.
function Section({
  to,
  icon: Icon,
  exact = false,
  mark,
  children,
}: {
  to: '/' | '/foods' | '/progress' | '/plan' | '/settings'
  icon: LucideIcon
  exact?: boolean
  mark?: string | undefined
  children: ReactNode
}) {
  const markId = useId()
  return (
    <Link
      to={to}
      // The day is current on any day's page, /?date=… too, and only
      // there, not on every path under /.
      activeOptions={exact ? { exact: true, includeSearch: false } : {}}
      aria-describedby={mark === undefined ? undefined : markId}
      className={sectionClass}
    >
      <span className="relative flex shrink-0">
        <Icon
          aria-hidden
          strokeWidth={1.75}
          className="size-5.5 lg:size-4.5 lg:text-muted-foreground lg:group-aria-[current=page]:text-primary"
        />
        {mark !== undefined && (
          <span
            aria-hidden
            className="absolute -top-0.5 -right-1 size-2.5 rounded-full bg-primary ring-2 ring-card lg:hidden"
          />
        )}
      </span>
      {children}
      {mark !== undefined && (
        <>
          <span aria-hidden className="ml-auto hidden size-2 rounded-full bg-primary lg:block" />
          <span id={markId} hidden>
            {mark}
          </span>
        </>
      )}
    </Link>
  )
}

// initials are a name's first letters, of its first two words.
function initials(name: string): string {
  return name
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((word) => Array.from(word)[0] ?? '')
    .join('')
    .toLocaleUpperCase()
}

// AppNav is the navigation between the app's sections, one for every
// width. On a phone it is a bar at the bottom, in reach of a thumb, with
// Log a meal in its middle; from 1024 px it is a sidebar, with Log a meal
// first, and the settings, the user and the theme at its foot. Each hides
// what is the other's. The plan's section has a dot while a proposal
// waits there for the user's answer.
export function AppNav() {
  const t = useMessages()
  const session = useQuery(sessionQuery)
  const name = session.data?.user.name ?? ''
  const waits = $api.useQuery('get', '/api/v1/plan').data?.pending !== undefined

  return (
    <nav
      aria-label={t.nav.label}
      className="fixed inset-x-0 bottom-0 z-30 border-t bg-card/95 pr-[env(safe-area-inset-right)] pb-[env(safe-area-inset-bottom)] pl-[env(safe-area-inset-left)] backdrop-blur-md lg:inset-y-0 lg:right-auto lg:flex lg:w-60 lg:flex-col lg:gap-6 lg:border-t-0 lg:border-r lg:bg-card lg:px-4 lg:py-6 lg:backdrop-blur-none"
    >
      <span className="hidden items-center gap-2.5 px-2 text-h3 lg:flex">
        <svg aria-hidden viewBox="0 0 32 32" className="size-7">
          <rect width="32" height="32" rx="9" className="fill-primary" />
          <path
            d="M8 20c3 0 4-8 8-8s5 6 8 6"
            fill="none"
            strokeWidth="2.5"
            strokeLinecap="round"
            className="stroke-primary-foreground"
          />
        </svg>
        tdeemon
      </span>
      <Link
        to="/"
        search={{ log: true }}
        aria-keyshortcuts="n"
        // cva joins classes without merging: cn lets hidden win over the
        // button's inline-flex.
        className={cn(buttonVariants(), 'hidden lg:flex')}
      >
        <Plus aria-hidden strokeWidth={2.25} />
        {t.nav.logMeal}
        <kbd
          aria-hidden
          className="rounded-[5px] border border-primary-foreground/40 px-1.5 font-mono text-caption opacity-80"
        >
          N
        </kbd>
      </Link>
      <ul className="grid grid-cols-5 items-center px-1 pt-1 lg:flex lg:flex-col lg:items-stretch lg:gap-1 lg:p-0">
        <li>
          <Section to="/" icon={CalendarDays} exact>
            {t.nav.today}
          </Section>
        </li>
        <li>
          <Section to="/foods" icon={Utensils}>
            {t.nav.foods}
          </Section>
        </li>
        <li className="flex justify-center lg:hidden">
          <Link
            to="/"
            search={{ log: true }}
            aria-label={t.nav.logMeal}
            className="flex size-13 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-[0_6px_16px_-4px] shadow-primary/50 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card"
          >
            <Plus aria-hidden strokeWidth={2.25} className="size-6" />
          </Link>
        </li>
        <li>
          <Section to="/progress" icon={ChartLine}>
            {t.nav.progress}
          </Section>
        </li>
        <li>
          <Section to="/plan" icon={Target} mark={waits ? t.nav.waits : undefined}>
            {t.nav.plan}
          </Section>
        </li>
      </ul>
      {/* On a phone the settings are the plan's, and the theme theirs. */}
      <div className="mt-auto hidden flex-col gap-1 lg:flex">
        <Section to="/settings" icon={Settings}>
          {t.nav.settings}
        </Section>
        <div className="mt-2 flex items-center gap-2.5 border-t px-3 pt-3">
          <span data-testid="user" className="flex min-w-0 flex-1 items-center gap-2.5">
            <span
              aria-hidden
              className="flex size-8 shrink-0 items-center justify-center rounded-full bg-surface-2 text-small font-semibold"
            >
              {initials(name)}
            </span>
            <span className="truncate text-small">{name}</span>
          </span>
          <ThemeToggle />
        </div>
      </div>
    </nav>
  )
}
