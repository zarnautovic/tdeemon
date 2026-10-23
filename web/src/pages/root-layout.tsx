import { Outlet } from '@tanstack/react-router'

// The frame every page renders in, the sign-in page too. Its sides keep
// clear of a phone's notch in landscape (viewport-fit=cover in index.html
// gives the page the whole screen).
export function RootLayout() {
  return (
    <div className="mx-auto min-h-dvh max-w-3xl pt-6 pr-[max(1rem,env(safe-area-inset-right))] pl-[max(1rem,env(safe-area-inset-left))]">
      <Outlet />
    </div>
  )
}
