import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { problem } from '@/i18n/problem'

// A change is what SaveStatus reads of a mutation.
interface Change<V> {
  isPending: boolean
  isError: boolean
  isSuccess: boolean
  error: unknown
  variables: V | undefined
  mutate: (variables: V) => void
}

// SaveStatus says where a change of the day stands: being saved, saved,
// or not saved and why, with a way to send it again; nothing is lost
// without a word.
export function SaveStatus<V>({
  change,
  onRetry,
}: {
  change: Change<V>
  // onRetry sends the change again; by default, as it was sent.
  onRetry?: () => void
}) {
  const t = useMessages()

  if (change.isError) {
    return (
      <span
        role="alert"
        className="inline-flex flex-wrap items-baseline gap-x-2 text-xs text-destructive"
      >
        {t.notSaved(problem(t, change.error))}
        <Button
          variant="link"
          size="xs"
          className="h-auto p-0"
          onClick={() => {
            if (onRetry) {
              onRetry()
            } else if (change.variables !== undefined) {
              change.mutate(change.variables)
            }
          }}
        >
          {t.retry}
        </Button>
      </span>
    )
  }
  return (
    <output className="text-xs text-muted-foreground">
      {change.isPending ? t.saving : change.isSuccess && t.saved}
    </output>
  )
}
