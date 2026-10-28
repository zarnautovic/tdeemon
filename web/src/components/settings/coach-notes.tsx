import { useQueryClient } from '@tanstack/react-query'
import { useState, type SubmitEvent } from 'react'

import { $api } from '@/api/client'
import { detailOf, statusOf } from '@/api/errors'
import type { components } from '@/api/schema'
import { SaveStatus } from '@/components/day/save-status'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { inputClass } from '@/lib/styles'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

type Me = components['schemas']['MeBody']

// maxNotes is the server's bound on the notes (profile.MaxNotesLen).
const maxNotes = 4000

// CoachNotes are what a coach, a model, should know of the user, in their
// words: get_profile gives them to it. labelledBy is the id of what names
// them, the page's title. What is typed stays in the field until it is
// saved, though the notes come back in between; saved, the field has the
// notes as the server has them, those an undo left too. A toast says they
// are saved, with Undo.
export function CoachNotes({ notes, labelledBy }: { notes: string; labelledBy: string }) {
  const t = useMessages()
  const queryClient = useQueryClient()
  // What is typed and not saved yet; undefined, the notes.
  const [draft, setDraft] = useState<string | undefined>(undefined)
  const save = $api.useMutation('put', '/api/v1/me/notes', {
    onSuccess: (answer) => {
      // What was sent is the notes now; what was typed since stays.
      setDraft((now) => (now?.trim() === answer.notes ? undefined : now))
      // The user has them, though asking for the user again fails.
      queryClient.setQueryData(
        $api.queryOptions('get', '/api/v1/me').queryKey,
        (me: Me | undefined) => me && { ...me, notes: answer.notes },
      )
      sayChange(
        { title: t.toast.notes },
        // Of no day: after the changes of today's, still on their way.
        { last: { action: 'notes.set', entity: 'notes' }, date: 'today' },
      )
      return queryClient.invalidateQueries({ queryKey: ['get', '/api/v1/me'] })
    },
  })

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    save.mutate({ body: { notes: (draft ?? notes).trim() } })
  }

  return (
    <form onSubmit={submit} className="space-y-4" aria-labelledby={labelledBy}>
      <textarea
        value={draft ?? notes}
        rows={8}
        maxLength={maxNotes}
        aria-labelledby={labelledBy}
        placeholder={t.settings.notesPlaceholder}
        className={cn(inputClass, 'h-auto py-3')}
        onChange={(e) => {
          setDraft(e.currentTarget.value)
        }}
      />
      <Button type="submit" disabled={save.isPending}>
        {t.settings.save}
      </Button>
      {save.isError && statusOf(save.error) === 422 ? (
        <p role="alert" className="text-sm text-destructive">
          {t.settings.refused(detailOf(save.error))}
        </p>
      ) : (
        <SaveStatus change={save} />
      )}
    </form>
  )
}
