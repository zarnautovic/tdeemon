import { Toast } from '@base-ui/react/toast'

import { uuidv7 } from '@/lib/uuid'

// A Change is a change of the user's as undo names it: what it did and
// what it changed (POST /api/v1/undo's last, and its answer's undone).
export interface Change {
  action: string
  entity: string
}

// Said is what a toast says: a change done, with what the server warned
// of it, the day it went to, and a way to take it back; or how an undo
// went.
export interface Said {
  title: string
  // What the server warned of the change, a line each.
  lines?: readonly string[] | undefined
  // A day the change is of, other than the one shown, linked to it.
  to?: string | undefined
  // The undo of the change: under an id made with the toast, so sent
  // again after no answer it takes back no other change; after the
  // changes of its day (date) still on their way.
  undo?: { last: Change; date: string; id: string } | undefined
  // A button of the toast's own, other than Undo: its words and what it
  // does.
  action?: { label: string; run: () => void } | undefined
}

// toasts is the app's toast manager: what any code adds, the Toasts
// component shows.
export const toasts = Toast.createToastManager<Said>()

// The toast of the latest change, one at a time: a change takes the place
// of the one before, so its Undo always takes back the latest change made
// here.
const changeToast = 'change'

// sayChange shows that a change is done: with Undo when undo takes it
// back (undo, the change and its day), longer when it says what the
// server warned of, or of a delete, which asked nothing first. Base UI
// keeps it while the pointer is over the toasts, the keyboard's focus is
// in them, or a finger touched them last.
export function sayChange(said: Omit<Said, 'undo'>, undo?: { last: Change; date: string }) {
  const long = (said.lines?.length ?? 0) > 0 || undo?.last.action.endsWith('.delete') === true
  toasts.add({
    id: changeToast,
    title: said.title,
    timeout: long ? 10_000 : 6000,
    data: { ...said, ...(undo && { undo: { ...undo, id: uuidv7() } }) },
  })
}

// sayUndone shows how an undo of the latest change went, in its toast.
export function sayUndone(said: Omit<Said, 'undo'>) {
  toasts.add({ id: changeToast, title: said.title, timeout: 6000, data: said })
}

// sayUpdate says a new version of the app is ready, in a toast of its own
// apart from the changes', which stays until the user takes it (action)
// or closes it.
export function sayUpdate(title: string, action: { label: string; run: () => void }) {
  toasts.add({ id: 'update', title, timeout: 0, data: { title, action } })
}
