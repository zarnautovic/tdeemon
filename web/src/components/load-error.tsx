import { CloudOff } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { problem } from '@/i18n/problem'
import { cn } from '@/lib/utils'

// LoadError is what a page, or a part of one, shows in place of its data
// when it did not come: what did not (title, a page's), why, and a way to
// ask again. A page's is a card where its content would be, under its
// header, the navigation around it as ever; a part's is in the part's
// card. Neither is in an error's red: nothing the user did is wrong.
export function LoadError({
  title,
  error,
  onRetry,
}: {
  title?: string
  error: unknown
  onRetry: () => void
}) {
  const t = useMessages()

  return (
    <div
      className={cn(
        'flex flex-col items-start gap-3',
        title !== undefined && 'rounded-lg border bg-card p-4 lg:p-6',
      )}
    >
      <div role="alert" className="flex gap-3">
        <CloudOff aria-hidden className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
        <div className="space-y-1">
          {title !== undefined && <p className="font-semibold">{title}</p>}
          <p className={cn(title !== undefined && 'text-small text-muted-foreground')}>
            {problem(t, error)}
          </p>
        </div>
      </div>
      <Button variant="outline" onClick={onRetry}>
        {t.retry}
      </Button>
    </div>
  )
}
