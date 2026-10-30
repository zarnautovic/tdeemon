import { Notifications } from '@/components/settings/notifications'
import { SubPage } from '@/components/sub-page'
import { useMessages } from '@/i18n/messages'

// NotificationsPage is the user's notifications: this device's, and every
// device that gets them.
export function NotificationsPage() {
  const t = useMessages()

  return (
    <SubPage
      title={t.settings.notifications.title}
      from="settings"
      hint={t.settings.notifications.hint}
    >
      <Notifications />
    </SubPage>
  )
}
