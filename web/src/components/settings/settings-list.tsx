import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Bell, Globe, History, LogOut, Palette, Smartphone } from 'lucide-react'
import { useState } from 'react'

import { $api, fetchClient } from '@/api/client'
import { HttpError } from '@/api/errors'
import { pushDeviceQuery } from '@/api/push'
import { sessionQuery } from '@/api/session'
import { ListGroup } from '@/components/list-group'
import { ListRow } from '@/components/list-row'
import { RecentChanges } from '@/components/recent-changes'
import { locales } from '@/i18n/locale'
import { useMessages } from '@/i18n/messages'
import { leave } from '@/lib/push'
import { useTheme } from '@/lib/theme'

// SettingsList is the user's settings, each a row to its page with its
// value in short: this browser's theme, the account's language, time zone
// and day's start, and the devices and AI clients with a token; the
// recent changes, in a sheet; then signing out, as whom, and what the
// server is. On the plan's page it has its heading; the settings' page is
// its title.
export function SettingsList({
  titled = false,
  className,
}: {
  titled?: boolean
  className?: string
}) {
  const t = useMessages()
  const theme = useTheme()
  const session = useQuery(sessionQuery)
  const user = session.data?.user
  const tokens = $api.useQuery('get', '/api/v1/tokens')
  const notified = $api.useQuery('get', '/api/v1/push/subscriptions')
  const info = $api.useQuery('get', '/api/v1/info')
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [changes, setChanges] = useState(false)
  const signOut = useMutation({
    // This device's notifications end first, in the session that ends.
    mutationFn: async () => {
      await leave(queryClient.getQueryData(pushDeviceQuery.queryKey))
      const { response } = await fetchClient.POST('/api/v1/auth/logout')
      if (!response.ok) {
        throw new HttpError(response.status)
      }
    },
    // Nothing of this user stays in the app for whoever signs in next.
    onSuccess: async () => {
      queryClient.clear()
      await navigate({ to: '/login', replace: true })
    },
  })
  const s = t.settings

  return (
    <ListGroup
      title={titled ? s.title : undefined}
      className={className}
      foot={
        <div className="space-y-1 px-1 text-small">
          {signOut.isError && (
            <p role="alert" className="text-destructive">
              {s.signOutFailed}
            </p>
          )}
          <p className="text-muted-foreground">
            {info.isPending && t.connecting}
            {info.isError && t.unreachable}
            {info.data && (
              <>
                <span data-testid="server-name">{info.data.name}</span>{' '}
                <span data-testid="server-version">{info.data.version}</span>
              </>
            )}
          </p>
        </div>
      }
    >
      <ListRow
        icon={Palette}
        title={s.appearance}
        to="/settings/appearance"
        summary={s.themes[theme]}
      />
      <ListRow
        icon={Globe}
        title={s.account}
        to="/settings/account"
        summary={
          user &&
          s.accountSummary(
            locales.find((l) => l.locale === user.locale)?.name ?? user.locale,
            user.timezone,
            `${String(user.day_start_hour).padStart(2, '0')}:00`,
          )
        }
      />
      <ListRow
        icon={Smartphone}
        title={s.devices}
        to="/settings/devices"
        summary={tokens.data && s.devicesSummary(tokens.data.tokens.length)}
      />
      <ListRow
        icon={Bell}
        title={s.notifications.title}
        to="/settings/notifications"
        summary={notified.data && s.notifications.summary(notified.data.subscriptions.length)}
      />
      <ListRow
        icon={History}
        title={t.changes.title}
        summary={t.changes.summary}
        onClick={() => {
          setChanges(true)
        }}
      />
      <RecentChanges open={changes} onOpenChange={setChanges} />
      <ListRow
        icon={LogOut}
        title={s.signOut}
        summary={user && s.signedInAs(user.name, user.email)}
        disabled={signOut.isPending}
        onClick={() => {
          signOut.mutate()
        }}
      />
    </ListGroup>
  )
}
