import { useNavigate, useSearch } from '@tanstack/react-router'
import { useEffect } from 'react'

import { ApiTokens } from '@/components/settings/api-tokens'
import { SubPage } from '@/components/sub-page'
import { useMessages } from '@/i18n/messages'

// DevicesPage is the devices and AI clients that sign in with a token of
// their own: the user's API tokens. /settings/devices?connect=coach opens
// a new token for an AI coach (the plan's Connect an AI coach).
export function DevicesPage() {
  const t = useMessages()
  const { connect } = useSearch({ from: '/app/settings/devices' })
  const navigate = useNavigate()
  useEffect(() => {
    if (connect === 'coach') {
      void navigate({ to: '/settings/devices', search: {}, replace: true })
    }
  }, [connect, navigate])

  return (
    <SubPage title={t.settings.devices} from="settings" hint={t.settings.devicesHint}>
      <ApiTokens coach={connect === 'coach'} />
    </SubPage>
  )
}
