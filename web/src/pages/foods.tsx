import { Outlet, useChildMatches } from '@tanstack/react-router'

import { FoodBrowser } from '@/components/foods/food-browser'
import { useDesktop } from '@/lib/media'

// FoodsPage is the user's foods (FoodBrowser) and the food open of them,
// or a new one: on a desktop side by side, the food in a pane that stays
// in view as the list scrolls; on a phone one or the other.
export function FoodsPage() {
  const desktop = useDesktop()
  // A food, or a new one, is open; else the foods alone.
  const open = useChildMatches({
    select: (matches) => matches.some((m) => m.routeId !== '/app/foods/'),
  })

  if (!desktop) {
    return <div className="mx-auto max-w-2xl">{open ? <Outlet /> : <FoodBrowser />}</div>
  }
  return (
    <div className="grid grid-cols-[minmax(0,26rem)_minmax(0,1fr)] items-start gap-6">
      <FoodBrowser />
      {/* Its cards' edges clear of where it scrolls. */}
      <div className="sticky top-8 -m-1 max-h-[calc(100dvh-4rem)] overflow-y-auto p-1">
        <Outlet />
      </div>
    </div>
  )
}
