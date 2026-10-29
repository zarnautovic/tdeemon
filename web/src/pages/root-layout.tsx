import { Outlet } from '@tanstack/react-router'

import { Toasts } from '@/components/toasts'
import { UpdatePrompt } from '@/components/update-prompt'

// The frame every page renders in, the sign-in page too: the whole
// screen, which the app's sections (AppLayout) and the pages outside them
// (signing in, not found) each lay out; and the toasts over it, a new
// version of the app among them.
export function RootLayout() {
  return (
    <div className="min-h-dvh">
      <Outlet />
      <Toasts />
      <UpdatePrompt />
    </div>
  )
}
