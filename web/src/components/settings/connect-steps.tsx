import { SquareCheck } from 'lucide-react'
import { useEffect, useRef, type ReactNode } from 'react'

import { $api } from '@/api/client'
import type { components } from '@/api/schema'
import { CopyField } from '@/components/copy-field'
import type { ConnectKind } from '@/components/settings/connect-kinds'
import { ConnectionState } from '@/components/settings/connection-state'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

type CreatedKey = components['schemas']['CreatedTokenBody']

// desktopConfig is Claude Desktop's configuration of tdeemon, through
// mcp-remote, which takes the header from a variable: Claude Desktop
// splits an argument at its spaces on some systems.
function desktopConfig(mcp: string, key: string): string {
  return JSON.stringify(
    {
      mcpServers: {
        tdeemon: {
          command: 'npx',
          // ${AUTH_HEADER} is mcp-remote's, not a template's.
          args: ['mcp-remote', mcp, '--header', 'Authorization:${AUTH_HEADER}'],
          env: { AUTH_HEADER: `Bearer ${key}` },
        },
      },
    },
    null,
    2,
  )
}

// ConnectSteps are the steps to give an access key just made to what it
// is for, the key shown this once. An AI coach's: the instance's MCP
// address, the key, Claude Code's command with both, a note each for
// Claude Desktop and ChatGPT, and a first message. A phone's: Health
// Auto Export's checklist, its address, method, header, metrics and
// Summarize Data. Another's: the key, and how it is sent. Then whether it
// connected, as it does, and Done. The address is the instance's public
// one, as the server says (its base URL), else the page's; the first
// Copy takes the focus.
export function ConnectSteps({
  kind,
  made,
  onDone,
}: {
  kind: ConnectKind
  made: CreatedKey
  onDone: () => void
}) {
  const t = useMessages()
  const info = $api.useQuery('get', '/api/v1/info')
  const base = info.isPending ? undefined : (info.data?.base_url ?? window.location.origin)

  return (
    <div className="space-y-4">
      {base === undefined ? (
        <output className="block text-muted-foreground">{t.loading}</output>
      ) : (
        <Steps kind={kind} made={made} base={base} />
      )}
      <ConnectionState id={made.id} />
      <Button variant="outline" className="w-full" onClick={onDone}>
        {t.settings.done}
      </Button>
    </div>
  )
}

// Steps are a kind's steps, at the instance's address base.
function Steps({ kind, made, base }: { kind: ConnectKind; made: CreatedKey; base: string }) {
  const t = useMessages()
  const c = t.connect
  const first = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    first.current?.focus()
  }, [])
  const mcp = `${base}/mcp`

  if (kind === 'coach') {
    return (
      <ol className="list-decimal space-y-4 pl-5 marker:text-small marker:text-muted-foreground">
        <li>
          <CopyField buttonRef={first} label={c.address} value={mcp} />
        </li>
        <li>
          <CopyField label={c.key} value={made.token} />
        </li>
        <li className="space-y-3">
          <CopyField
            label={c.claudeCode}
            value={`claude mcp add --transport http tdeemon ${mcp} --header "Authorization: Bearer ${made.token}"`}
          />
          <CopyField label={c.claudeDesktop} value={desktopConfig(mcp, made.token)} lines />
          <p className="text-small text-muted-foreground">{c.chatGpt}</p>
        </li>
        <li>
          <CopyField label={c.first} value={c.message} />
        </li>
      </ol>
    )
  }
  if (kind === 'phone') {
    return (
      <div className="space-y-3">
        <p className="text-small">{c.hae.intro}</p>
        <ul className="space-y-3">
          <Check>
            <CopyField buttonRef={first} label={c.hae.url} value={`${base}/api/v1/ingest/hae`} />
          </Check>
          <Check>{c.hae.method}</Check>
          <Check>
            <CopyField label={c.hae.header} value={`Bearer ${made.token}`} />
          </Check>
          <Check>{c.hae.metrics}</Check>
          <Check>{c.hae.summarize}</Check>
        </ul>
      </div>
    )
  }
  return (
    <div className="space-y-2">
      <CopyField buttonRef={first} label={c.key} value={made.token} />
      <p className="text-small text-muted-foreground">{c.header}</p>
    </div>
  )
}

// Check is a line of a checklist.
function Check({ children }: { children: ReactNode }) {
  return (
    <li className="flex gap-3">
      <SquareCheck aria-hidden strokeWidth={1.75} className="mt-0.5 size-5 shrink-0 text-primary" />
      <div className="min-w-0 flex-1 text-small">{children}</div>
    </li>
  )
}
