import { useId, useState } from 'react'

import { $api } from '@/api/client'
import { LoadError } from '@/components/load-error'
import { CoachNotes } from '@/components/settings/coach-notes'
import { Skeleton } from '@/components/skeleton'
import { SubPage } from '@/components/sub-page'
import { useMessages } from '@/i18n/messages'

// NotesPage is what a coach, a model, should know of the user, in their
// words: get_profile gives them to it.
export function NotesPage() {
  const t = useMessages()
  const id = useId()
  const me = $api.useQuery('get', '/api/v1/me')
  // The user as the server gave them since the page opened, not before.
  const [opened] = useState(Date.now)
  const fresh = me.data !== undefined && me.dataUpdatedAt >= opened

  return (
    <SubPage title={t.settings.notes} titleId={id} from="plan" hint={t.settings.notesHint}>
      {/* The notes as the server has them when the page opens, not as a
          cache kept them: an undo or another tab may have changed them,
          and a failed answer leaves the cache's. Once shown, what is typed
          stays, saved or not, whatever a later ask of the user comes to. */}
      {fresh ? (
        <CoachNotes notes={me.data.notes} labelledBy={id} />
      ) : me.isError ? (
        <LoadError error={me.error} onRetry={() => void me.refetch()} />
      ) : (
        <Skeleton blocks={['h-36', 'h-11 w-28']} />
      )}
    </SubPage>
  )
}
