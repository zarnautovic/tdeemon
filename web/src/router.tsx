import type { QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  type RouterHistory,
} from '@tanstack/react-router'

import { AppLayout } from '@/pages/app-layout'
import { ChartsPage } from '@/pages/charts'
import { DayPage } from '@/pages/day'
import { FoodsPage } from '@/pages/foods'
import { NotFoundPage } from '@/pages/not-found'
import { RootLayout } from '@/pages/root-layout'
import { SettingsPage } from '@/pages/settings'

// Routes are declared in code, not generated from files: the app has few
// screens, and this keeps type-safe routing without a generated route tree.

interface RouterContext {
  queryClient: QueryClient
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
  notFoundComponent: NotFoundPage,
})

// The app's sections share a layout with the navigation between them; a
// page outside them, as the not-found page, has the root's frame only.
const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app',
  component: AppLayout,
})

const dayRoute = createRoute({ getParentRoute: () => appRoute, path: '/', component: DayPage })
const foodsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/foods',
  component: FoodsPage,
})
const chartsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/charts',
  component: ChartsPage,
})
const settingsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/settings',
  component: SettingsPage,
})

const routeTree = rootRoute.addChildren([
  appRoute.addChildren([dayRoute, foodsRoute, chartsRoute, settingsRoute]),
])

export function createAppRouter(options: { queryClient: QueryClient; history?: RouterHistory }) {
  return createRouter({
    routeTree,
    context: { queryClient: options.queryClient },
    ...(options.history ? { history: options.history } : {}),
    defaultPreload: 'intent',
  })
}

export type AppRouter = ReturnType<typeof createAppRouter>

declare module '@tanstack/react-router' {
  interface Register {
    router: AppRouter
  }
}
