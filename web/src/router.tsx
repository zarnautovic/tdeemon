import { hashKey, type QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
  type RouterHistory,
} from '@tanstack/react-router'

import { statusOf } from '@/api/errors'
import { safeTarget, sessionQuery } from '@/api/session'
import { isDate } from '@/i18n/format'
import { setLocale } from '@/i18n/locale'
import { AppError } from '@/pages/app-error'
import { AppLayout } from '@/pages/app-layout'
import { ChartsPage } from '@/pages/charts'
import { DayPage } from '@/pages/day'
import { FoodsPage } from '@/pages/foods'
import { LoginPage } from '@/pages/login'
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

// The app's sections share a layout with the navigation between them,
// and are for a signed-in user: without a session they send the browser
// to the sign-in page, which brings it back. The user's language is the
// app's. A page outside them (signing in, not found) has the root's frame
// only.
const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app',
  beforeLoad: async ({ context, location }) => {
    try {
      const session = await context.queryClient.query(sessionQuery)
      setLocale(session.user.locale)
      return { session }
    } catch (error) {
      if (statusOf(error) === 401) {
        // oxlint-disable-next-line typescript/only-throw-error -- the router's redirect is thrown by design
        throw redirect({ to: '/login', search: { redirect: location.href } })
      }
      throw error
    }
  },
  component: AppLayout,
})

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: (search: Record<string, unknown>): { redirect?: string } =>
    typeof search.redirect === 'string' ? { redirect: search.redirect } : {},
  // A signed-in user goes straight where they were headed. Without an
  // answer from the server, the page shows (and says why it cannot help).
  beforeLoad: async ({ context, search }) => {
    let signedIn = false
    try {
      await context.queryClient.query(sessionQuery)
      signedIn = true
    } catch {
      // Not signed in, or no answer.
    }
    if (signedIn) {
      // oxlint-disable-next-line typescript/only-throw-error -- the router's redirect is thrown by design
      throw redirect({ href: safeTarget(search.redirect), replace: true })
    }
  },
  component: LoginPage,
})

// The day's date, YYYY-MM-DD; without it, or with one that is no date
// (2026-02-31), today.
const dayRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/',
  // The router keeps the URL's keys a validator leaves out, so a date
  // that is none is set undefined, not left out.
  validateSearch: (search: Record<string, unknown>): { date?: string | undefined } => ({
    date: typeof search.date === 'string' && isDate(search.date) ? search.date : undefined,
  }),
  component: DayPage,
})
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
  loginRoute,
])

export function createAppRouter(options: { queryClient: QueryClient; history?: RouterHistory }) {
  const router = createRouter({
    routeTree,
    context: { queryClient: options.queryClient },
    // An error a route does not handle, the sections' session check's
    // above all, in the app's words and within the root's frame, not the
    // router's own page.
    defaultErrorComponent: AppError,
    ...(options.history ? { history: options.history } : {}),
    defaultPreload: 'intent',
  })
  // A query or a change refused for want of a session (it ended, or was
  // ended elsewhere) sends the browser to sign in: the sections ask the
  // server again who is signed in, instead of the answer they keep for a
  // minute. The session's own 401 is theirs to handle. (A sign-in's 401,
  // a wrong password, only has the sign-in page ask again: no session.)
  const signedOut = (error: unknown) => {
    if (statusOf(error) === 401) {
      options.queryClient.removeQueries({ queryKey: sessionQuery.queryKey })
      void router.invalidate()
    }
  }
  options.queryClient.getQueryCache().subscribe((event) => {
    if (
      event.type === 'updated' &&
      event.action.type === 'error' &&
      event.query.queryHash !== sessionHash
    ) {
      signedOut(event.action.error)
    }
  })
  options.queryClient.getMutationCache().subscribe((event) => {
    if (event.type === 'updated' && event.action.type === 'error') {
      signedOut(event.action.error)
    }
  })
  return router
}

const sessionHash = hashKey(sessionQuery.queryKey)

export type AppRouter = ReturnType<typeof createAppRouter>

declare module '@tanstack/react-router' {
  interface Register {
    router: AppRouter
  }
}
