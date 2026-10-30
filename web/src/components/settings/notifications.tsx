import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BellRing, Smartphone } from 'lucide-react'

import { $api } from '@/api/client'
import { pushDeviceQuery } from '@/api/push'
import { sessionQuery } from '@/api/session'
import { LoadError } from '@/components/load-error'
import { Skeleton } from '@/components/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { turnOff, turnOn, type State } from '@/lib/push'

const subscriptions = ['get', '/api/v1/push/subscriptions']

// Notifications are the user's notifications by Web Push: this device's,
// turned on (the browser asks for the permission at the tap) or off, or
// why it cannot have them (a browser without push, an iPhone's Safari
// outside the app installed, a permission refused); every device that
// gets them, each removed apart; and a test sent to them all.
export function Notifications() {
  const t = useMessages()
  const n = t.settings.notifications
  const f = useFormat()
  const queryClient = useQueryClient()
  const session = useQuery(sessionQuery)
  const timeZone = session.data?.user.timezone ?? 'UTC'
  const device = useQuery(pushDeviceQuery)
  const list = $api.useQuery('get', '/api/v1/push/subscriptions')
  const changed = async (state: State) => {
    queryClient.setQueryData(pushDeviceQuery.queryKey, state)
    await queryClient.invalidateQueries({ queryKey: subscriptions })
  }
  const toggle = useMutation({
    mutationFn: (state: State) => (state.kind === 'on' ? turnOff(state.id) : turnOn()),
    onSuccess: changed,
  })
  const remove = $api.useMutation('delete', '/api/v1/push/subscriptions/{id}', {
    onSettled: async (_, __, vars) => {
      // This device's own, removed in the list, is off here too.
      if (device.data?.kind === 'on' && device.data.id === vars.params.path.id) {
        queryClient.setQueryData(pushDeviceQuery.queryKey, { kind: 'off' })
      }
      await queryClient.invalidateQueries({ queryKey: subscriptions })
    },
  })
  const test = $api.useMutation('post', '/api/v1/push/test', {
    onSettled: () => queryClient.invalidateQueries({ queryKey: subscriptions }),
  })

  const state = device.data
  let here
  if (state === undefined) {
    here = device.isError ? <p className="text-small">{n.failed}</p> : <Skeleton blocks={['h-6']} />
  } else if (state.kind === 'unsupported' || state.kind === 'install' || state.kind === 'blocked') {
    here = <p className="text-small text-muted-foreground">{n[state.kind]}</p>
  } else {
    here = (
      <div className="flex items-center justify-between gap-3">
        <p aria-live="polite" className="text-small text-muted-foreground">
          {state.kind === 'on' ? n.on : n.off}
        </p>
        <Button
          variant={state.kind === 'on' ? 'outline' : 'default'}
          disabled={toggle.isPending}
          onClick={() => {
            toggle.mutate(state)
          }}
        >
          {state.kind === 'on' ? n.turnOff : n.turnOn}
        </Button>
      </div>
    )
  }

  let devices
  if (list.isPending) {
    devices = <Skeleton blocks={['h-16']} />
  } else if (list.data === undefined) {
    devices = <LoadError title={n.devices} error={list.error} onRetry={() => void list.refetch()} />
  } else if (list.data.subscriptions.length === 0) {
    devices = <p className="text-small text-muted-foreground">{n.noDevices}</p>
  } else {
    devices = (
      <ul className="space-y-3">
        {list.data.subscriptions.map((d) => (
          <li
            key={d.id}
            className="flex items-center gap-3 rounded-lg border bg-card p-4 shadow-card"
          >
            <span
              aria-hidden
              className="flex size-9 shrink-0 items-center justify-center rounded-md bg-surface-2 text-muted-foreground"
            >
              <Smartphone strokeWidth={1.75} className="size-4.5" />
            </span>
            <div className="min-w-0 flex-1">
              <h3 className="flex flex-wrap items-center gap-2 font-semibold">
                <span className="truncate">{d.device}</span>
                {state?.kind === 'on' && state.id === d.id && (
                  <Badge tone="primary">{n.thisDevice}</Badge>
                )}
              </h3>
              <p className="text-small text-muted-foreground">
                {d.last_sent_at
                  ? n.lastSent(
                      `${f.day(d.last_sent_at, timeZone)} ${f.time(d.last_sent_at, timeZone)}`,
                    )
                  : n.neverSent}
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              aria-label={n.remove(d.device)}
              disabled={remove.isPending}
              onClick={() => {
                remove.mutate({ params: { path: { id: d.id } } })
              }}
            >
              {n.removeShort}
            </Button>
          </li>
        ))}
      </ul>
    )
  }

  let tested
  if (test.isError) {
    tested = n.testFailed
  } else if (test.data) {
    tested =
      test.data.sent === 0 && test.data.failed === 0
        ? n.testNone
        : n.tested(test.data.sent, test.data.failed)
  }

  return (
    <div className="space-y-6">
      <section
        aria-labelledby="this-device"
        className="space-y-3 rounded-lg border bg-card p-4 shadow-card"
      >
        <h2 id="this-device" className="flex items-center gap-2 font-semibold">
          <BellRing aria-hidden strokeWidth={1.75} className="size-4.5" />
          {n.thisDevice}
        </h2>
        {here}
        {toggle.isError && (
          <p role="alert" className="text-small">
            {n.failed}
          </p>
        )}
      </section>
      <section aria-labelledby="notified" className="space-y-3">
        <h2 id="notified" className="font-semibold">
          {n.devices}
        </h2>
        {devices}
        {list.isSuccess && list.data.subscriptions.length > 0 && (
          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant="outline"
              disabled={test.isPending}
              onClick={() => {
                test.mutate({})
              }}
            >
              {n.test}
            </Button>
            <output className="text-small text-muted-foreground">{tested}</output>
          </div>
        )}
      </section>
    </div>
  )
}
