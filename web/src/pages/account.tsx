import { useQuery } from '@tanstack/react-query'
import { useId } from 'react'

import { sessionQuery } from '@/api/session'
import { YouForm } from '@/components/settings/you-form'
import { SubPage } from '@/components/sub-page'
import { useMessages } from '@/i18n/messages'

// AccountPage changes the user's name, the app's language, their time
// zone and the hour their day starts.
export function AccountPage() {
  const t = useMessages()
  const id = useId()
  const session = useQuery(sessionQuery)

  return (
    <SubPage title={t.settings.account} titleId={id} from="settings">
      {/* The sections have the session before they show. */}
      {session.data && (
        <YouForm key={session.data.user.id} user={session.data.user} labelledBy={id} />
      )}
    </SubPage>
  )
}
