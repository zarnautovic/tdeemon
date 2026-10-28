import { Appearance } from '@/components/settings/appearance'
import { SubPage } from '@/components/sub-page'
import { useMessages } from '@/i18n/messages'

// AppearancePage picks this browser's theme: the system's, light or dark.
export function AppearancePage() {
  const t = useMessages()

  return (
    <SubPage title={t.settings.appearance} from="settings">
      <div className="rounded-lg border bg-card p-4 lg:p-5">
        <Appearance />
      </div>
    </SubPage>
  )
}
