import { useLocation, useSearch } from '@tanstack/react-router'

import { $api } from '@/api/client'
import { statusOf } from '@/api/errors'
import { useUndo } from '@/api/meals'
import type { Messages } from '@/i18n/en'
import { useMessages } from '@/i18n/messages'
import { problem } from '@/i18n/problem'
import { sayUndone, type Change } from '@/lib/toast'
import { uuidv7 } from '@/lib/uuid'

// undoRefused is what an undo that took nothing back says, by its answer:
// nothing to take back (404), a change that clashes with what came since
// (409), another change made since the one named (412), or no answer.
export function undoRefused(t: Messages, error: unknown): string {
  const status = statusOf(error)
  if (status === 404) {
    return t.undo.nothing
  }
  if (status === 409) {
    return t.undo.clash
  }
  if (status === 412) {
    return t.undo.notLatest
  }
  return t.undo.failed(problem(t, error))
}

// useShownDay is the day a day's page shows, its date or today; none on
// another page.
export function useShownDay(): string | undefined {
  const onDay = useLocation({ select: (l) => l.pathname === '/' })
  const date = useSearch({ strict: false, select: (s) => s.date })
  const today = $api.useQuery('get', '/api/v1/me').data?.today
  return onDay ? (date ?? today) : undefined
}

// useUndoLatest takes back the user's latest change, made anywhere, or
// the one it names (last) while that is the latest, and says how it went
// in the toast of changes: what it took back, linking the day it was of
// when another is shown, or why it took nothing back. Each undo is sent
// under an id of its own; one the server refuses (made by a clock far
// from its own) goes again without, and the server makes one.
export function useUndoLatest() {
  const t = useMessages()
  const shown = useShownDay()
  const run = useUndo(shown ?? 'today')

  function send(last: Change | undefined, id: string | undefined) {
    run.mutate(
      { body: { ...(id !== undefined && { id }), ...(last !== undefined && { last }) } },
      {
        onSuccess: ({ undone }) => {
          sayUndone({
            title: t.undo.done(t.undo.actions[undone.action] ?? t.undo.change),
            to: undone.days.find((d) => d !== shown),
          })
        },
        onError: (error) => {
          if (statusOf(error) === 422 && id !== undefined) {
            send(last, undefined)
            return
          }
          sayUndone({ title: undoRefused(t, error) })
        },
      },
    )
  }

  return {
    undo: (last?: Change) => {
      send(last, uuidv7())
    },
    pending: run.isPending,
  }
}
