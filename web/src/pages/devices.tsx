import { ApiTokens } from '@/components/settings/api-tokens'
import { SubPage } from '@/components/sub-page'
import { useMessages } from '@/i18n/messages'

// DevicesPage is the devices and AI clients that sign in with a token of
// their own: the user's API tokens.
export function DevicesPage() {
  const t = useMessages()

  return (
    <SubPage title={t.settings.devices} from="settings" hint={t.settings.devicesHint}>
      <ApiTokens />
    </SubPage>
  )
}
