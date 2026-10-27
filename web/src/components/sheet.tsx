import { Dialog } from '@base-ui/react/dialog'
import { X } from 'lucide-react'
import type { ReactNode, RefObject } from 'react'

import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// Sheet is a panel over the page, titled: from the bottom on a phone, at
// the right on a desktop, sliding in on the sheet's curve (with less
// motion asked for, it only fades). Escape, its close button and a tap
// beside it close it, and the focus goes back where it was. What it holds
// is there while it is open, so it starts anew each time. initialFocus is
// the field the focus goes to when it opens; finalFocus, where it goes
// back to, when that is not what had it (a tap on a button gives it no
// focus in Safari). A tall one takes most of a
// phone's height, whatever it holds, so what it lists does not move its
// field up and down. A bare one leaves its space under the title to what
// it holds, which lays out what scrolls and what does not (its foot).
export function Sheet({
  open,
  onOpenChange,
  title,
  initialFocus,
  finalFocus,
  tall = false,
  bare = false,
  children,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  initialFocus?: RefObject<HTMLElement | null>
  finalFocus?: RefObject<HTMLElement | null> | undefined
  tall?: boolean
  bare?: boolean
  children: ReactNode
}) {
  const t = useMessages()

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop className="fixed inset-0 z-40 bg-black/40 transition-opacity duration-(--duration-slow) data-ending-style:opacity-0 data-starting-style:opacity-0" />
        <Dialog.Popup
          initialFocus={initialFocus}
          finalFocus={finalFocus}
          className={cn(
            'fixed inset-x-0 bottom-0 z-50 flex max-h-[90dvh] flex-col rounded-t-xl bg-card shadow-sheet transition-transform duration-(--duration-slow) ease-sheet outline-none data-ending-style:translate-y-full data-starting-style:translate-y-full lg:inset-y-0 lg:right-0 lg:left-auto lg:max-h-none lg:w-md lg:rounded-l-xl lg:rounded-tr-none lg:data-ending-style:translate-x-full lg:data-ending-style:translate-y-0 lg:data-starting-style:translate-x-full lg:data-starting-style:translate-y-0',
            tall && 'h-[90dvh] lg:h-auto',
          )}
        >
          <div className="flex items-center justify-between gap-2 border-b py-2 pr-2 pl-4">
            <Dialog.Title className="text-h3">{title}</Dialog.Title>
            <Dialog.Close
              aria-label={t.close}
              className={buttonVariants({ variant: 'ghost', size: 'icon-sm' })}
            >
              <X aria-hidden />
            </Dialog.Close>
          </div>
          {bare ? (
            children
          ) : (
            <div className="overflow-y-auto p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
              {children}
            </div>
          )}
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
