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
import { FoodPage } from '@/pages/food'
import { FoodsPage } from '@/pages/foods'
import { LoginPage } from '@/pages/login'
import { NewFoodPage } from '@/pages/new-food'
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
// The foods: a search (q), or the list in use or archived, by name or by
// the last use.
const foodsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/foods',
  validateSearch: (search: Record<string, unknown>): FoodsSearch => ({
    q: typeof search.q === 'string' && search.q !== '' ? search.q : undefined,
    archived: search.archived === true || search.archived === 'true' ? true : undefined,
    order: search.order === 'use' ? 'use' : undefined,
  }),
  component: FoodsPage,
})
const newFoodRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/foods/new',
  component: NewFoodPage,
})
const foodRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/foods/$id',
  component: FoodPage,
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
  appRoute.addChildren([dayRoute, foodsRoute, newFoodRoute, foodRoute, chartsRoute, settingsRoute]),
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

// FoodsSearch is what the foods page shows: its search, its list and its
// order.
export interface FoodsSearch {
  q?: string | undefined
  archived?: true | undefined
  order?: 'use' | undefined
}

declare module '@tanstack/history' {
  interface HistoryState {
    // The foods page a food's page was opened from, to go back to.
    foods?: FoodsSearch
  }
}
