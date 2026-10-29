import { Toast, type ToastObject } from '@base-ui/react/toast'
import { Link } from '@tanstack/react-router'
import { X } from 'lucide-react'

import { $api } from '@/api/client'
import { statusOf } from '@/api/errors'
import { useUndo } from '@/api/meals'
import { Button, buttonVariants } from '@/components/ui/button'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import { sayUndone, toasts, type Said } from '@/lib/toast'
import { undoRefused, useShownDay } from '@/lib/undo'

// Toasts shows the app's toasts, announced as they come (aria-live, by
// Base UI): above the bar on a phone, at the bottom right on a desktop.
// They are over a sheet too (z-50, its portal after theirs), so a change
// made in one is taken back from there; a sheet keeps Tab within it, and
// F6 takes the focus to them (Base UI), as a sheet leaves them out of
// what it hides. Rendered within the router, so a toast links a day.
export function Toasts() {
  const t = useMessages()

  return (
    <Toast.Provider toastManager={toasts}>
      <Toast.Portal>
        <Toast.Viewport
          aria-label={t.toast.region}
          className="fixed inset-x-4 bottom-[calc(6rem+env(safe-area-inset-bottom))] z-60 mx-auto flex max-w-md flex-col gap-2 outline-none lg:right-8 lg:bottom-8 lg:left-auto lg:mx-0 lg:w-96"
        >
          <List />
        </Toast.Viewport>
      </Toast.Portal>
    </Toast.Provider>
  )
}

function List() {
  const { toasts: shown } = Toast.useToastManager<Said>()
  return shown.map((toast) => <Item key={toast.id} toast={toast} />)
}

function Item({ toast }: { toast: ToastObject<Said> }) {
  const t = useMessages()
  const said = toast.data

  return (
    <Toast.Root
      toast={toast}
      className="flex items-start gap-2 rounded-lg border bg-card py-2 pr-2 pl-4 shadow-sheet transition-[opacity,translate] duration-(--duration-base) data-ending-style:opacity-0 data-starting-style:translate-y-2 data-starting-style:opacity-0"
    >
      <Toast.Content className="min-w-0 flex-1 space-y-1 py-1.5">
        {/* Not a heading: a toast is no part of the page's outline. */}
        <Toast.Title render={<p />} className="text-sm font-medium" />
        {said?.lines !== undefined && said.lines.length > 0 && (
          <ul className="text-sm text-warning">
            {said.lines.map((line, i) => (
              // Two items may bring a warning of one code.
              <li key={`${line} ${String(i)}`}>{line}</li>
            ))}
          </ul>
        )}
        {said?.to !== undefined && <DayLink date={said.to} />}
      </Toast.Content>
      {said?.undo !== undefined && <UndoAction undo={said.undo} />}
      <Toast.Close
        aria-label={t.toast.close}
        className={buttonVariants({ variant: 'ghost', size: 'icon-sm' })}
      >
        <X aria-hidden />
      </Toast.Close>
    </Toast.Root>
  )
}

// DayLink is the way to a day a change is of, other than the one shown.
function DayLink({ date }: { date: string }) {
  const t = useMessages()
  const f = useFormat()
  const today = $api.useQuery('get', '/api/v1/me').data?.today

  return (
    <Link
      to="/"
      // Today's link is / itself.
      search={date === today ? {} : { date }}
      className="block text-sm text-primary underline underline-offset-2"
    >
      {date === today ? t.meal.seeToday : t.toast.see(f.date(date, today))}
    </Link>
  )
}

// UndoAction takes back the change of its toast, under the toast's id, so
// a tap again after no answer takes back no other; and only while it is
// the latest change (last): after another, made elsewhere, nothing. The
// toast then says how it went, linking the day the change was of when
// another is shown.
function UndoAction({ undo }: { undo: NonNullable<Said['undo']> }) {
  const t = useMessages()
  const run = useUndo(undo.date)
  const shown = useShownDay()
  const said = (error: unknown) => undoRefused(t, error)

  function send(id: string | undefined) {
    run.mutate(
      { body: { ...(id !== undefined && { id }), last: undo.last } },
      {
        onSuccess: ({ undone }) => {
          sayUndone({
            title: t.undo.done(t.undo.actions[undone.action] ?? t.undo.change),
            to: undone.days.find((d) => d !== shown),
          })
        },
        onError: (error) => {
          const status = statusOf(error)
          // An id the server refuses was made by a clock far from its
          // own: it makes one then.
          if (status === 422 && id !== undefined) {
            send(undefined)
            return
          }
          // Without an answer, the same undo may be sent again: under
          // its id, it takes back no other change.
          const again = id !== undefined && (status === undefined || status >= 500)
          toasts.update('change', {
            title: said(error),
            timeout: again ? 0 : 6000,
            data: again ? { title: said(error), undo } : { title: said(error) },
          })
        },
      },
    )
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      className="font-semibold text-primary"
      disabled={run.isPending}
      onClick={() => {
        // The toast stays while the undo is on its way.
        toasts.update('change', { timeout: 0 })
        send(undo.id)
      }}
    >
      {t.toast.undo}
    </Button>
  )
}
