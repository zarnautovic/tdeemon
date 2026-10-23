import { Link } from '@tanstack/react-router'
import { CalendarDays, ChartLine, Settings, Utensils } from 'lucide-react'

import { useMessages } from '@/i18n/messages'

const linkClass =
  'flex flex-col items-center gap-1 py-2 text-xs text-muted-foreground outline-none focus-visible:text-foreground focus-visible:underline aria-[current=page]:text-foreground'

// SectionNav links the app's sections; the current one is marked
// aria-current="page" (TanStack Router's Link does it).
export function SectionNav() {
  const t = useMessages()

  return (
    <nav
      aria-label={t.nav.label}
      className="fixed inset-x-0 bottom-0 border-t bg-background pb-[env(safe-area-inset-bottom)]"
    >
      <ul className="mx-auto grid max-w-3xl grid-cols-4">
        <li>
          <Link to="/" activeOptions={{ exact: true }} className={linkClass}>
            <CalendarDays aria-hidden className="size-5" />
            {t.nav.day}
          </Link>
        </li>
        <li>
          <Link to="/foods" className={linkClass}>
            <Utensils aria-hidden className="size-5" />
            {t.nav.foods}
          </Link>
        </li>
        <li>
          <Link to="/charts" className={linkClass}>
            <ChartLine aria-hidden className="size-5" />
            {t.nav.charts}
          </Link>
        </li>
        <li>
          <Link to="/settings" className={linkClass}>
            <Settings aria-hidden className="size-5" />
            {t.nav.settings}
          </Link>
        </li>
      </ul>
    </nav>
  )
}
