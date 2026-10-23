import { Undo2 } from 'lucide-react'

import { statusOf } from '@/api/errors'
import { useUndo } from '@/api/meals'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { uuidv7 } from '@/lib/uuid'

// UndoButton takes back the user's latest change, made here or anywhere
// else, and says what it took back. Each tap is an undo of its own, under
// a new id; trying again after no answer sends the same one, so it takes
// back no more than one change.
export function UndoButton({ date }: { date: string }) {
  const t = useMessages()
  const undo = useUndo(date)
  const status = statusOf(undo.error)

  let said
  if (undo.isSuccess) {
    const action = undo.data.undone.action
    said = (
      <output className="text-sm">{t.undo.done(t.undo.actions[action] ?? t.undo.change)}</output>
    )
  } else if (undo.isError && status === 404) {
    said = <output className="text-sm text-muted-foreground">{t.undo.nothing}</output>
  } else if (undo.isError && status === 409) {
    said = (
      <p role="alert" className="text-sm text-destructive">
        {t.undo.clash}
      </p>
    )
  } else if (undo.isError) {
    said = <SaveStatus change={undo} />
  }

  return (
    <div className="space-y-1">
      <Button
        variant="outline"
        size="lg"
        className="w-full"
        disabled={undo.isPending}
        onClick={() => {
          undo.mutate({ body: { id: uuidv7() } })
        }}
      >
        <Undo2 aria-hidden />
        {t.undo.button}
      </Button>
      {said}
    </div>
  )
}
