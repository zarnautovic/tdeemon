import { Popover } from '@base-ui/react/popover'
import { Info, X } from 'lucide-react'
import type { ReactNode } from 'react'

import { buttonVariants } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { cn } from '@/lib/utils'

// InfoTip is an ⓘ beside a number, which says what the number is in a
// few sentences (children), under its name (title): a popover by it, on
// a phone as on a desktop, kept within the screen. The ⓘ is a button
// named label, the size of the text to the eye and 44 px to a finger (an
// ::after over it); a tap, Enter or Space opens it. Open, it is a modal
// dialog to a screen reader, named by its title and described by its
// sentences, the focus on its close button and kept within it; the close
// button, a tap beside it or Escape closes it, the focus back on the ⓘ.
export function InfoTip({
  label,
  title,
  className,
  children,
}: {
  label: string
  title: string
  className?: string
  children: ReactNode
}) {
  const t = useMessages()

  return (
    <Popover.Root modal>
      <Popover.Trigger
        aria-label={label}
        className={cn(
          "relative inline-flex size-5 shrink-0 items-center justify-center rounded-full text-muted-foreground outline-none after:absolute after:-inset-3 after:content-[''] hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-popup-open:text-foreground",
          className,
        )}
      >
        <Info aria-hidden className="size-4" />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner sideOffset={8} collisionPadding={16} className="z-40">
          <Popover.Popup className="w-80 max-w-(--available-width) rounded-lg border bg-popover py-1 pr-1 pl-3 text-small font-normal text-popover-foreground shadow-sheet transition-opacity duration-(--duration-base) outline-none data-ending-style:opacity-0 data-starting-style:opacity-0">
            <div className="flex items-center justify-between gap-2">
              <Popover.Title className="text-small font-semibold">{title}</Popover.Title>
              <Popover.Close
                aria-label={t.close}
                className={buttonVariants({ variant: 'ghost', size: 'icon-sm' })}
              >
                <X aria-hidden />
              </Popover.Close>
            </div>
            <Popover.Description className="pr-2 pb-2 text-small font-normal text-muted-foreground">
              {children}
            </Popover.Description>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  )
}
