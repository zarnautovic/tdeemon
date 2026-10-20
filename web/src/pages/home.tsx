import { $api } from '@/api/client'

export function HomePage() {
  const info = $api.useQuery('get', '/api/v1/info')

  return (
    <section className="space-y-2">
      <h1 className="text-2xl font-semibold">tdeemon</h1>
      {info.isPending && <p className="text-muted-foreground">Connecting to the server…</p>}
      {info.isError && <p className="text-destructive">The server cannot be reached.</p>}
      {info.data && (
        <p className="text-muted-foreground">
          <span data-testid="server-name">{info.data.name}</span>{' '}
          <span data-testid="server-version">{info.data.version}</span>
        </p>
      )}
    </section>
  )
}
