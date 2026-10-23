import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'
import { useState } from 'react'

import { $api } from '@/api/client'
import { sessionQuery } from '@/api/session'
import { LoadError } from '@/components/load-error'
import { ApiTokens } from '@/components/settings/api-tokens'
import { CoachNotes } from '@/components/settings/coach-notes'
import { YouForm } from '@/components/settings/you-form'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// SettingsPage is the user's own: their name, language, time zone and
// day's start, their notes for a coach, the way to their profile and
// limits, their API tokens, signing out, and what the server is.
export function SettingsPage() {
  const t = useMessages()
  const session = useQuery(sessionQuery)
  const info = $api.useQuery('get', '/api/v1/info')
  const me = $api.useQuery('get', '/api/v1/me')
  // The user as the server gave them since the page opened, not before.
  const [opened] = useState(Date.now)
  const fresh = me.data !== undefined && me.dataUpdatedAt >= opened
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const signOut = $api.useMutation('post', '/api/v1/auth/logout', {
    // Nothing of this user stays in the app for whoever signs in next.
    onSuccess: async () => {
      queryClient.clear()
      await navigate({ to: '/login', replace: true })
    },
  })

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold">{t.settings.title}</h1>
      {session.data && <YouForm key={session.data.user.id} user={session.data.user} />}
      {/* The notes as the server has them when the page opens, not as a
          cache kept them: an undo or another tab may have changed them,
          and a failed answer leaves the cache's. Once shown, what is typed
          stays, saved or not, whatever a later ask of the user comes to. */}
      {fresh ? (
        <CoachNotes notes={me.data.notes} />
      ) : (
        me.isError && <LoadError error={me.error} onRetry={() => void me.refetch()} />
      )}
      <Link
        to="/settings/plan"
        className="flex items-center justify-between gap-2 rounded-lg border p-3 hover:bg-muted"
      >
        <span>
          <span className="block font-medium">{t.plan.open}</span>
          <span className="block text-sm text-muted-foreground">{t.plan.openHint}</span>
        </span>
        <ChevronRight aria-hidden className="shrink-0" />
      </Link>
      <ApiTokens />
      <section className="space-y-2">
        <h2 className="font-medium">{t.settings.account}</h2>
        {session.data && (
          <p className="text-muted-foreground">
            {t.settings.signedInAs(session.data.user.name, session.data.user.email)}
          </p>
        )}
        {signOut.isError && (
          <p role="alert" className="text-destructive">
            {t.settings.signOutFailed}
          </p>
        )}
        <Button
          variant="outline"
          disabled={signOut.isPending}
          onClick={() => {
            signOut.mutate({})
          }}
        >
          {t.settings.signOut}
        </Button>
      </section>
      <section className="space-y-1">
        <h2 className="font-medium">{t.settings.about}</h2>
        {info.isPending && <p className="text-muted-foreground">{t.connecting}</p>}
        {info.isError && <p className="text-destructive">{t.unreachable}</p>}
        {info.data && (
          <p className="text-muted-foreground">
            <span data-testid="server-name">{info.data.name}</span>{' '}
            <span data-testid="server-version">{info.data.version}</span>
          </p>
        )}
      </section>
    </section>
  )
}
