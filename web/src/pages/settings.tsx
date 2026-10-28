import { Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'

import { SettingsList } from '@/components/settings/settings-list'
import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// SettingsPage is the user's settings, each a row to its page: this
// browser's theme, the account's language, time zone and day's start, the
// devices and AI clients, and signing out. A desktop's sidebar leads here;
// a phone has them on the plan's page, and goes back to it.
export function SettingsPage() {
  const t = useMessages()

  return (
    <section className="max-w-2xl space-y-4">
      <Link
        to="/plan"
        className={buttonVariants({ variant: 'ghost', className: '-ml-3 lg:hidden' })}
      >
        <ChevronLeft aria-hidden />
        {t.nav.plan}
      </Link>
      <h1 className="text-title">{t.settings.title}</h1>
      <SettingsList />
    </section>
  )
}
