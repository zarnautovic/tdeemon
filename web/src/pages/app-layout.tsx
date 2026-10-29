import { Outlet, useNavigate } from '@tanstack/react-router'
import { useRef, useState } from 'react'

import { AppNav } from '@/components/app-nav'
import { ShortcutsDialog } from '@/components/shortcuts-dialog'
import { useMessages } from '@/i18n/messages'
import { useShortcuts } from '@/lib/shortcuts'
import { useUndoLatest } from '@/lib/undo'

// The frame of the app's sections: the navigation between them (a bar at
// the bottom on a phone, a sidebar on a desktop) and the page, whose end
// keeps clear of the bar and whose sides of a phone's notch in landscape
// (viewport-fit=cover in index.html gives the page the whole screen).
// The keyboard's first stop skips the sections to the page. The keyboard
// has a shortcut to log a meal (N), to find a food (/), to undo the
// latest change (⌘Z or Ctrl+Z, which a toast says), to each section (G,
// then its letter), and to the list of them (?).
export function AppLayout() {
  const t = useMessages()
  const navigate = useNavigate()
  const main = useRef<HTMLElement>(null)
  const [shortcuts, setShortcuts] = useState(false)
  const latest = useUndoLatest()
  useShortcuts({
    n: () => {
      void navigate({ to: '/', search: { log: true } })
    },
    '/': () => {
      // The foods' search, there already or once there.
      const field = globalThis.document.querySelector<HTMLElement>('[data-find]')
      if (field) {
        field.focus()
      } else {
        void navigate({ to: '/foods', state: { find: true } })
      }
    },
    'Mod+z': () => {
      latest.undo()
    },
    // The sections, by the key after G: D for the day, F the foods, P the
    // progress, L the plan, S the settings.
    'g d': () => {
      void navigate({ to: '/' })
    },
    'g f': () => {
      void navigate({ to: '/foods' })
    },
    'g p': () => {
      void navigate({ to: '/progress' })
    },
    'g l': () => {
      void navigate({ to: '/plan' })
    },
    'g s': () => {
      void navigate({ to: '/settings' })
    },
    '?': () => {
      setShortcuts(true)
    },
  })

  return (
    <>
      <a
        href="#content"
        onClick={(e) => {
          // The page's start takes the focus, and the address stays the
          // page's.
          e.preventDefault()
          main.current?.focus()
        }}
        // Out of sight until it has the focus: its box is the sight's.
        className="sr-only z-50 rounded-md bg-card font-medium text-primary shadow-sheet focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:flex focus:min-h-11 focus:items-center focus:px-4 focus:ring-2 focus:ring-ring focus:outline-none"
      >
        {t.nav.skip}
      </a>
      <AppNav />
      <main id="content" ref={main} tabIndex={-1} className="pb-28 outline-none lg:pb-12 lg:pl-60">
        <div className="mx-auto w-full max-w-300 pt-6 pr-[max(1rem,env(safe-area-inset-right))] pl-[max(1rem,env(safe-area-inset-left))] lg:px-10 lg:pt-8">
          <Outlet />
        </div>
      </main>
      <ShortcutsDialog open={shortcuts} onOpenChange={setShortcuts} />
    </>
  )
}
