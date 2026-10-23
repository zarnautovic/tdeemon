import { Outlet } from '@tanstack/react-router'

// The frame every page renders in, the sign-in page too.
export function RootLayout() {
  return (
    <div className="mx-auto min-h-dvh max-w-3xl px-4 pt-6">
      <Outlet />
    </div>
  )
}
