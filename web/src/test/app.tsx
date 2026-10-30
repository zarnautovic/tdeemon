import { QueryClient } from '@tanstack/react-query'
import { createMemoryHistory } from '@tanstack/react-router'
import { act, render } from '@testing-library/react'
import { vi } from 'vitest'

import type { components } from '@/api/schema'
import type { Day } from '@/api/types'
import { App } from '@/app'
import type { Locale } from '@/i18n/locale'
import { createAppRouter, type AppRouter } from '@/router'

// renderAt renders the whole app at a path, and returns its router to see
// where it went.
export function renderAt(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter({
    queryClient,
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  render(<App router={router} queryClient={queryClient} />)
  return router
}

export type Handler = (request: Request) => Response | Promise<Response>

// fakeServer stands in for the API at fetch: a handler per "METHOD /path",
// and a 404 problem for anything else. It returns the stub, to see what
// was asked.
export function fakeServer(routes: Record<string, Handler>) {
  const fetch = vi.fn<typeof globalThis.fetch>(async (input, init) => {
    const request = input instanceof Request ? input : new Request(input, init)
    const handler = routes[`${request.method} ${new URL(request.url).pathname}`]
    return handler ? handler(request) : problem(404)
  })
  vi.stubGlobal('fetch', fetch)
  return fetch
}

// unreachable takes the server out of reach: no request gets an answer,
// as when the network is gone.
export function unreachable() {
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof globalThis.fetch>(() => Promise.reject(new TypeError('Failed to fetch'))),
  )
}

// askAgain asks the server again for all the page shows, as a window back
// in focus does, and waits for the answers.
export async function askAgain(router: AppRouter) {
  await act(() => router.options.context.queryClient.invalidateQueries())
}

// problem is an error answer of the API (RFC 9457).
export function problem(status: number, headers: Record<string, string> = {}): Response {
  return Response.json(
    { status, title: 'An error', detail: 'what went wrong' },
    { status, headers: { 'Content-Type': 'application/problem+json', ...headers } },
  )
}

export function session(locale: Locale = 'en'): components['schemas']['SessionBody'] {
  return {
    user: {
      id: '0193c3a5-0000-7000-8000-000000000001',
      email: 'ana@example.com',
      name: 'Ana',
      role: 'user',
      locale,
      // Far from a developer's own, so a time shown in the machine's zone,
      // not the user's, fails the tests anywhere.
      timezone: 'America/Los_Angeles',
      day_start_hour: 4,
    },
    expires_at: '2026-11-22T10:00:00Z',
  }
}

export const today = '2026-10-23'

// day is a day with nothing logged, its target that of a rest day; over
// is laid over it.
export function day(over: Partial<Day> = {}): Day {
  return {
    date: today,
    status: 'open',
    type: 'rest',
    target: { kcal: 2500, protein: 160, carbs: 280, fat: 80 },
    total: { kcal: 0, protein: 0, carbs: 0, fat: 0 },
    remaining: { kcal: 2500, protein: 160, carbs: 280, fat: 80 },
    meals: [],
    metrics: [],
    ...over,
  }
}

// me is who the user is, as GET /api/v1/me answers, today being today.
export function me(): components['schemas']['MeBody'] {
  return {
    name: 'Ana',
    locale: 'en',
    time_zone: 'America/Los_Angeles',
    day_start_hour: 4,
    now: `${today}T09:30:00-07:00`,
    today,
    today_type: 'rest',
    notes: '',
    profile: { missing: [] },
    training_weekdays: [],
    units: { energy: 'kcal', nutrients: 'g', weight: 'kg', height: 'cm' },
  }
}

// signedIn are the routes of a server with a session, whose user has
// logged nothing today.
export function signedIn(locale: Locale = 'en'): Record<string, Handler> {
  return {
    'GET /api/v1/auth/session': () => Response.json(session(locale)),
    'GET /api/v1/me': () => Response.json(me()),
    'GET /api/v1/days/today': () => Response.json(day()),
    'GET /api/v1/tokens': () => Response.json({ tokens: [] }),
    'GET /api/v1/push/subscriptions': () => Response.json({ subscriptions: [] }),
  }
}

// signedOut are the routes of a server without one, which offers a
// password and an OpenID Connect provider.
export const signedOut: Record<string, Handler> = {
  'GET /api/v1/auth/session': () => problem(401),
  'GET /api/v1/auth/methods': () =>
    Response.json({
      password: true,
      oidc: { name: 'Authentik', login_url: '/api/v1/auth/oidc/login' },
    }),
}
