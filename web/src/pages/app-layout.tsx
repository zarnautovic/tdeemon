import { Outlet, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'

import { AppNav } from '@/components/app-nav'
import { ShortcutsDialog } from '@/components/shortcuts-dialog'
import { useShortcuts } from '@/lib/shortcuts'

// The frame of the app's sections: the navigation between them (a bar at
// the bottom on a phone, a sidebar on a desktop) and the page, whose end
// keeps clear of the bar and whose sides of a phone's notch in landscape
// (viewport-fit=cover in index.html gives the page the whole screen).
// The keyboard has a shortcut to log a meal (N), to find a food (/), and
// to the list of them (?).
export function AppLayout() {
  const navigate = useNavigate()
  const [shortcuts, setShortcuts] = useState(false)
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
    '?': () => {
      setShortcuts(true)
    },
  })

  return (
    <>
      <AppNav />
      <main className="pb-28 lg:pb-12 lg:pl-60">
        <div className="mx-auto w-full max-w-300 pt-6 pr-[max(1rem,env(safe-area-inset-right))] pl-[max(1rem,env(safe-area-inset-left))] lg:px-10 lg:pt-8">
          <Outlet />
        </div>
      </main>
      <ShortcutsDialog open={shortcuts} onOpenChange={setShortcuts} />
    </>
  )
}
