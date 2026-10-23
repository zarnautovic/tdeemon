import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { problem } from '@/i18n/problem'

// LoadError is what a part of a page shows when its data did not come,
// with a way to ask again.
export function LoadError({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const t = useMessages()

  return (
    <div className="space-y-2">
      <p role="alert" className="text-destructive">
        {problem(t, error)}
      </p>
      <Button variant="outline" onClick={onRetry}>
        {t.retry}
      </Button>
    </div>
  )
}
