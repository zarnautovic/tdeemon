import { Sunrise } from 'lucide-react'

import { $api } from '@/api/client'
import { LoadError } from '@/components/load-error'
import { Sheet } from '@/components/sheet'
import { Skeleton } from '@/components/skeleton'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// MorningButton opens this morning's summary (MorningSummary) from the
// day's header, today's: what the morning's notification says, whole, for
// a notification gone or never had.
export function MorningButton({ onOpen }: { onOpen: () => void }) {
  const t = useMessages()

  return (
    <Button variant="ghost" size="icon-sm" aria-label={t.day.morning.open} onClick={onOpen}>
      <Sunrise aria-hidden />
    </Button>
  )
}

// MorningSummary is the morning summary of date, whole, in a sheet: what
// a tap on the morning's notification opens (/?summary=date), as the
// server reads it now, a part a line, its name beside what it says. The
// server's first line, the title and the date, is the sheet's.
export function MorningSummary({
  date,
  onClose,
}: {
  date: string | undefined
  onClose: () => void
}) {
  const t = useMessages()
  const open = date !== undefined
  const summary = $api.useQuery(
    'get',
    '/api/v1/morning',
    { params: { query: { date: date ?? '' } } },
    { enabled: open },
  )

  let body
  if (summary.data !== undefined) {
    const [heading, ...lines] = summary.data.text.split('\n')
    body = (
      <div className="space-y-4">
        <p className="text-small text-muted-foreground">{heading}</p>
        <dl className="space-y-3">
          {lines.map((line) => {
            const at = line.indexOf(': ')
            return (
              <div key={line} className="space-y-0.5">
                <dt className="text-small font-semibold">{at < 0 ? line : line.slice(0, at)}</dt>
                {at >= 0 && <dd>{line.slice(at + 2)}</dd>}
              </div>
            )
          })}
        </dl>
      </div>
    )
  } else if (summary.isError) {
    body = (
      <LoadError
        title={t.day.morning.failed}
        error={summary.error}
        onRetry={() => void summary.refetch()}
      />
    )
  } else {
    body = <Skeleton blocks={['h-5', 'h-12', 'h-12', 'h-12']} />
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(o) => {
        if (!o) {
          onClose()
        }
      }}
      title={summary.data?.title ?? t.day.morning.title}
    >
      {body}
    </Sheet>
  )
}
