import { Outlet } from '@tanstack/react-router'

// The frame every page renders in.
export function RootLayout() {
  return (
    <div className="mx-auto min-h-dvh max-w-3xl px-4 py-8">
      <main>
        <Outlet />
      </main>
    </div>
  )
}
