import { Outlet } from '@tanstack/react-router'

import { SectionNav } from '@/components/section-nav'

// The frame of the app's sections: the page, and the navigation between
// sections fixed at the bottom, in reach of a thumb on a phone. The
// padding keeps the page's end clear of it.
export function AppLayout() {
  return (
    <>
      <main className="pb-24">
        <Outlet />
      </main>
      <SectionNav />
    </>
  )
}
