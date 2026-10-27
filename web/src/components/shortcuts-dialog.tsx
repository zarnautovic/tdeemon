import { Dialog } from '@base-ui/react/dialog'

import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

const kbdClass =
  'inline-flex min-w-7 items-center justify-center rounded-sm border bg-surface-2 px-1.5 font-mono text-small'

// ShortcutsDialog lists the keyboard's shortcuts (?), each with its keys.
// Escape closes it, and the focus goes back where it was.
export function ShortcutsDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const t = useMessages()
  const shortcuts = [
    { keys: ['N'], what: t.shortcuts.logMeal },
    { keys: ['/'], what: t.shortcuts.find },
    { keys: ['←', '→'], what: t.shortcuts.days },
    { keys: ['?'], what: t.shortcuts.list },
  ]

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop className="fixed inset-0 z-40 bg-black/40 transition-opacity duration-(--duration-base) data-ending-style:opacity-0 data-starting-style:opacity-0" />
        <Dialog.Popup className="fixed top-1/2 left-1/2 z-50 w-[min(24rem,calc(100vw-2rem))] -translate-1/2 space-y-4 rounded-xl bg-card p-6 shadow-sheet transition-opacity duration-(--duration-slow) ease-sheet data-ending-style:opacity-0 data-starting-style:opacity-0">
          <Dialog.Title className="text-h2">{t.shortcuts.title}</Dialog.Title>
          <dl className="space-y-2">
            {shortcuts.map(({ keys, what }) => (
              <div key={what} className="flex items-center justify-between gap-4">
                <dt>{what}</dt>
                <dd className="flex gap-1">
                  {keys.map((key) => (
                    <kbd key={key} className={kbdClass}>
                      {key}
                    </kbd>
                  ))}
                </dd>
              </div>
            ))}
          </dl>
          <Dialog.Close className={buttonVariants({ variant: 'outline', className: 'w-full' })}>
            {t.shortcuts.close}
          </Dialog.Close>
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
