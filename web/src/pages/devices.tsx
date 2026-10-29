import { useNavigate, useSearch } from '@tanstack/react-router'
import { useEffect } from 'react'

import { ApiTokens } from '@/components/settings/api-tokens'
import { SubPage } from '@/components/sub-page'
import { useMessages } from '@/i18n/messages'

// DevicesPage is the devices and AI coaches connected, each with an
// access key of its own: the user's API tokens. ?connect= opens the way
// to connect another, at an AI coach (the plan's Connect an AI coach), a
// phone, or the choice (today's first steps).
export function DevicesPage() {
  const t = useMessages()
  const { connect } = useSearch({ from: '/app/settings/devices' })
  const navigate = useNavigate()
  useEffect(() => {
    if (connect !== undefined) {
      void navigate({ to: '/settings/devices', search: {}, replace: true })
    }
  }, [connect, navigate])

  return (
    <SubPage title={t.settings.devices} from="settings" hint={t.settings.devicesHint}>
      <ApiTokens connect={connect} />
    </SubPage>
  )
}
