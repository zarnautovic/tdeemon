import { hashKey, type QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  defaultStringifySearch,
  lazyRouteComponent,
  redirect,
  type RouterHistory,
} from '@tanstack/react-router'

import { statusOf } from '@/api/errors'
import { safeTarget, sessionQuery } from '@/api/session'
import { isDate } from '@/i18n/format'
import { setLocale } from '@/i18n/locale'
import limits from '@/lib/limits.json'
import { AccountPage } from '@/pages/account'
import { AppError } from '@/pages/app-error'
import { AppLayout } from '@/pages/app-layout'
import { AppearancePage } from '@/pages/appearance'
import { DayPage } from '@/pages/day'
import { DevicesPage } from '@/pages/devices'
import { FoodPage } from '@/pages/food'
import { FoodsPage } from '@/pages/foods'
import { FoodsIndexPage } from '@/pages/foods-index'
import { LimitsPage } from '@/pages/limits'
import { LoginPage } from '@/pages/login'
import { NewFoodPage } from '@/pages/new-food'
import { NotFoundPage } from '@/pages/not-found'
import { NotesPage } from '@/pages/notes'
import { NotificationsPage } from '@/pages/notifications'
import { PlanPage } from '@/pages/plan'
import { ProfilePage } from '@/pages/profile'
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
      // Without an answer, or with the server failing, a section opens on
      // the session the app has: its page says what did not come, within
      // the app's frame.
      const known = context.queryClient.getQueryData(sessionQuery.queryKey)
      if (known !== undefined) {
        return { session: known }
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

// The day's date, YYYY-MM-DD; without it, with one that is no date
// (2026-02-31), or one before the first day a history is of (1900-01-01,
// limits.json), today. log opens the form to log a meal (the app's Log a
// meal), which takes it out of the address once open.
const dayRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/',
  // The router keeps the URL's keys a validator leaves out, so a date
  // that is none is set undefined, not left out.
  validateSearch: (search: Record<string, unknown>): DaySearch => ({
    date:
      typeof search.date === 'string' && isDate(search.date) && search.date >= limits.history.since
        ? search.date
        : undefined,
    log: search.log === true || search.log === 'true' ? true : undefined,
  }),
  component: DayPage,
})
// The foods: a search (q), or a list (in use, archived or the estimates),
// by how many meals had them lately, by the last use or by name; and
// within it a food, or a new one, beside the list on a desktop and
// instead of it on a phone. The list's search goes with the food's
// address, so the list beside it, and back from it, is as it was left.
// archived=true, the archive's address before the lists, leads to the
// archive's address now, so a list chosen from then on is the filter's.
const foodsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/foods',
  beforeLoad: ({ location, search }) => {
    if ('archived' in location.search) {
      // Its own fields alone: the router keeps what it did not validate.
      const { q, filter, order } = search
      // oxlint-disable-next-line typescript/only-throw-error -- the router's redirect is thrown by design
      throw redirect({
        href: `${location.pathname}${defaultStringifySearch({ q, filter, order })}`,
        replace: true,
      })
    }
  },
  validateSearch: (search: Record<string, unknown>): FoodsSearch => ({
    q: typeof search.q === 'string' && search.q !== '' ? search.q : undefined,
    filter:
      search.filter === 'archived' || search.filter === 'estimates'
        ? search.filter
        : search.archived === true || search.archived === 'true'
          ? 'archived'
          : undefined,
    order: search.order === 'use' || search.order === 'name' ? search.order : undefined,
  }),
  component: FoodsPage,
})
const foodsIndexRoute = createRoute({
  getParentRoute: () => foodsRoute,
  path: '/',
  component: FoodsIndexPage,
})
const newFoodRoute = createRoute({
  getParentRoute: () => foodsRoute,
  path: 'new',
  component: NewFoodPage,
})
// A food, read; edit=true corrects it.
const foodRoute = createRoute({
  getParentRoute: () => foodsRoute,
  path: '$id',
  validateSearch: (search: Record<string, unknown>): { edit?: true | undefined } => ({
    edit: search.edit === true || search.edit === 'true' ? true : undefined,
  }),
  component: FoodPage,
})
// The progress, over the span of days the address says (90 by default),
// or the phase's days: its charts load ECharts with them, not with the
// app.
function spanOf(search: Record<string, unknown>): {
  days?: 30 | 180 | 365 | 'phase' | undefined
} {
  if (search.days === 'phase') {
    return { days: 'phase' }
  }
  const days = Number(search.days)
  return { days: days === 30 || days === 180 || days === 365 ? days : undefined }
}
const progressRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/progress',
  validateSearch: spanOf,
  component: lazyRouteComponent(() => import('@/pages/charts'), 'ChartsPage'),
})
// The plan, and what it rests on of the user's, a page each: the
// profile, the limits and the notes for a coach.
const planRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/plan',
  component: PlanPage,
})
const profileRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/plan/profile',
  component: ProfilePage,
})
const limitsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/plan/limits',
  component: LimitsPage,
})
const notesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/plan/notes',
  component: NotesPage,
})
// The settings, as a list, and a page each: the theme, the account, and
// the devices and AI clients.
const settingsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/settings',
  component: SettingsPage,
})
const appearanceRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/settings/appearance',
  component: AppearancePage,
})
const accountRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/settings/account',
  component: AccountPage,
})
// connect opens the way to connect an AI coach (coach, the plan's Connect
// an AI coach), a phone (phone), or either (choose, today's first steps),
// which takes it out of the address once open.
const devicesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/settings/devices',
  validateSearch: (
    search: Record<string, unknown>,
  ): { connect?: 'coach' | 'phone' | 'choose' | undefined } => ({
    connect:
      search.connect === 'coach' || search.connect === 'phone' || search.connect === 'choose'
        ? search.connect
        : undefined,
  }),
  component: DevicesPage,
})
const notificationsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/settings/notifications',
  component: NotificationsPage,
})
// The addresses of before, the charts' and the plan's under the settings,
// lead where they are now.
const chartsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/charts',
  validateSearch: spanOf,
  beforeLoad: ({ search }) => {
    // oxlint-disable-next-line typescript/only-throw-error -- the router's redirect is thrown by design
    throw redirect({ to: '/progress', search, replace: true })
  },
})
const oldPlanRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/plan',
  beforeLoad: () => {
    // oxlint-disable-next-line typescript/only-throw-error -- the router's redirect is thrown by design
    throw redirect({ to: '/plan', replace: true })
  },
})

const routeTree = rootRoute.addChildren([
  appRoute.addChildren([
    dayRoute,
    foodsRoute.addChildren([foodsIndexRoute, newFoodRoute, foodRoute]),
    progressRoute,
    planRoute,
    profileRoute,
    limitsRoute,
    notesRoute,
    settingsRoute,
    appearanceRoute,
    accountRoute,
    devicesRoute,
    notificationsRoute,
  ]),
  loginRoute,
  chartsRoute,
  oldPlanRoute,
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

// DaySearch is the day a day's page shows, and whether to log a meal.
export interface DaySearch {
  date?: string | undefined
  log?: true | undefined
}

// FoodsSearch is what the foods page shows: its search, its list (in use
// when left out) and its order (by how many meals had them lately when
// left out).
export interface FoodsSearch {
  q?: string | undefined
  filter?: 'archived' | 'estimates' | undefined
  order?: 'use' | 'name' | undefined
}

declare module '@tanstack/history' {
  interface HistoryState {
    // The foods' search takes the focus (the shortcut /).
    find?: true
    // A food's form was opened from the food, which it goes back to.
    fromFood?: true
  }
}
