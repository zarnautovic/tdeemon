import { Outlet } from '@tanstack/react-router'

// The frame every page renders in, the sign-in page too: the whole
// screen, which the app's sections (AppLayout) and the pages outside them
// (signing in, not found) each lay out.
export function RootLayout() {
  return (
    <div className="min-h-dvh">
      <Outlet />
    </div>
  )
}
