import { $api } from '@/api/client'
import { useMessages } from '@/i18n/messages'

export function SettingsPage() {
  const t = useMessages()
  const info = $api.useQuery('get', '/api/v1/info')

  return (
    <section className="space-y-4">
      <h1 className="text-2xl font-semibold">{t.settings.title}</h1>
      <section className="space-y-1">
        <h2 className="font-medium">{t.settings.about}</h2>
        {info.isPending && <p className="text-muted-foreground">{t.settings.connecting}</p>}
        {info.isError && <p className="text-destructive">{t.settings.unreachable}</p>}
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
