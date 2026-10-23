import { useQueryClient } from '@tanstack/react-query'
import { useId, useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { inputClass } from '@/lib/styles'
import { cn } from '@/lib/utils'

// maxNotes is the server's bound on the notes (profile.MaxNotesLen).
const maxNotes = 4000

// CoachNotes are what a coach, a model, should know of the user, in their
// words: get_me gives them to it.
export function CoachNotes({ notes }: { notes: string }) {
  const t = useMessages()
  const id = useId()
  const queryClient = useQueryClient()
  const [text, setText] = useState(notes)
  const save = $api.useMutation('put', '/api/v1/me/notes', {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/me'] }),
  })

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    save.mutate({ body: { notes: text.trim() } })
  }

  return (
    <form onSubmit={submit} className="space-y-2">
      <h2 className="font-medium">
        <label htmlFor={id}>{t.settings.notes}</label>
      </h2>
      <p id={`${id}-hint`} className="text-sm text-muted-foreground">
        {t.settings.notesHint}
      </p>
      <textarea
        id={id}
        value={text}
        rows={5}
        maxLength={maxNotes}
        aria-describedby={`${id}-hint`}
        className={cn(inputClass, 'h-auto py-2')}
        onChange={(e) => {
          setText(e.currentTarget.value)
        }}
      />
      {save.isError && statusOf(save.error) === 422 ? (
        <p role="alert" className="text-sm text-destructive">
          {t.settings.refused(detailOf(save.error))}
        </p>
      ) : (
        <SaveStatus change={save} />
      )}
      <Button type="submit" disabled={save.isPending}>
        {t.settings.save}
      </Button>
    </form>
  )
}
