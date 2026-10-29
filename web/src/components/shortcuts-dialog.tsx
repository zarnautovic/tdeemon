import { Dialog } from '@base-ui/react/dialog'
import { Fragment } from 'react'

import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { apple } from '@/lib/shortcuts'

const kbdClass =
  'inline-flex min-w-7 items-center justify-center rounded-sm border bg-surface-2 px-1.5 font-mono text-small text-foreground'

// A shortcut: its keys, pressed together (+), one after the other (then),
// or either of them, and what it does.
interface Shortcut {
  keys: string[]
  join?: '+' | 'then'
  what: string
}

// go is the shortcut to a section: G, then its letter.
function go(letter: string, what: string): Shortcut {
  return { keys: ['G', letter], join: 'then', what }
}

// ShortcutsDialog lists the keyboard's shortcuts (?), each with its keys:
// those of anywhere, the sections' (G, then a letter) and a day's, its
// meals' among them. Escape closes it, and the focus goes back where it
// was.
export function ShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const t = useMessages()
  const s = t.shortcuts
  const groups: { title: string; shortcuts: Shortcut[] }[] = [
    {
      title: s.anywhere,
      shortcuts: [
        { keys: ['N'], what: s.logMeal },
        { keys: ['/'], what: s.find },
        { keys: [apple() ? '⌘' : 'Ctrl', 'Z'], join: '+', what: s.undo },
        // Base UI's key, which reaches them from a sheet too, as Tab does not.
        { keys: ['F6'], what: s.toasts },
        { keys: ['?'], what: s.list },
      ],
    },
    {
      title: s.goTo,
      shortcuts: [
        go('D', s.sections.today),
        go('F', s.sections.foods),
        go('P', s.sections.progress),
        go('L', s.sections.plan),
        go('S', s.sections.settings),
      ],
    },
    {
      title: s.onDay,
      shortcuts: [
        { keys: ['←', '→'], what: s.days },
        { keys: ['↑', '↓'], what: s.meals },
        { keys: ['Enter'], what: s.change },
        { keys: ['R'], what: s.repeat },
        { keys: ['Delete'], what: s.remove },
      ],
    },
  ]

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop className="fixed inset-0 z-40 bg-black/40 transition-opacity duration-(--duration-base) data-ending-style:opacity-0 data-starting-style:opacity-0" />
        <Dialog.Popup className="fixed top-1/2 left-1/2 z-50 max-h-[90dvh] w-[min(26rem,calc(100vw-2rem))] -translate-1/2 space-y-4 overflow-y-auto rounded-xl bg-card p-6 shadow-sheet transition-opacity duration-(--duration-slow) ease-sheet data-ending-style:opacity-0 data-starting-style:opacity-0">
          <Dialog.Title className="text-h2">{s.title}</Dialog.Title>
          {groups.map((group) => (
            <section key={group.title} className="space-y-2">
              <h3 className="text-small font-semibold text-muted-foreground">{group.title}</h3>
              <dl className="space-y-2">
                {group.shortcuts.map(({ keys, join, what }) => (
                  <div key={what} className="flex items-center justify-between gap-4">
                    <dt>{what}</dt>
                    <dd className="flex items-center gap-1 text-small text-muted-foreground">
                      {keys.map((key, i) => (
                        <Fragment key={key}>
                          {i > 0 && join !== undefined && (
                            <span>{join === 'then' ? s.andThen : '+'}</span>
                          )}
                          <kbd className={kbdClass}>{key}</kbd>
                        </Fragment>
                      ))}
                    </dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
          <Dialog.Close className={buttonVariants({ variant: 'outline', className: 'w-full' })}>
            {s.close}
          </Dialog.Close>
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
