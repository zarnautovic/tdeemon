import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, session, signedIn, signedOut } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

// server is a signed-out server whose sign-in answers with login: a
// sign-in that succeeds starts the session the app asks for next.
function server(login: (request: Request) => Response | Promise<Response>) {
  let live = false
  return fakeServer({
    ...signedIn(),
    ...signedOut,
    'GET /api/v1/auth/session': () => (live ? Response.json(session('hr')) : problem(401)),
    'POST /api/v1/auth/login': async (request) => {
      const answer = await login(request)
      live = answer.ok
      return answer
    },
  })
}

async function signIn(email = 'ana@example.com', password = 'correct horse') {
  fireEvent.change(await screen.findByLabelText('Email'), { target: { value: email } })
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: password } })
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
}

test('a section without a session sends the browser to sign in, and back after it', async () => {
  const fetch = server(() => Response.json(session('hr')))
  const router = renderAt('/foods?from=nav')

  expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeDefined()
  expect(router.state.location.pathname).toBe('/login')
  expect(screen.queryByRole('navigation')).toBeNull()

  await signIn()

  // The user's language is the app's from then on.
  expect(await screen.findByRole('heading', { name: 'Hrana' })).toBeDefined()
  expect(router.state.location.href).toBe('/foods?from=nav')
  expect(localStorage.getItem('tdeemon.locale')).toBe('hr')
  const login = fetch.mock.calls
    .map(([request]) => request)
    .find((r) => r instanceof Request && r.method === 'POST')
  expect(login instanceof Request && (await login.json())).toEqual({
    email: 'ana@example.com',
    password: 'correct horse',
  })
})

test("the provider's sign-in is a link that brings the browser back", async () => {
  server(() => problem(401))

  renderAt('/charts')

  const link = await screen.findByRole('link', { name: 'Sign in with Authentik' })
  expect(link.getAttribute('href')).toBe('/api/v1/auth/oidc/login?return_to=%2Fcharts')
})

test('a target off the app is the start page after signing in', async () => {
  server(() => Response.json(session()))
  const router = renderAt('/login?redirect=%2F%2Fevil.example%2F')

  expect(
    (await screen.findByRole('link', { name: 'Sign in with Authentik' })).getAttribute('href'),
  ).toBe('/api/v1/auth/oidc/login?return_to=%2F')
  await signIn()

  expect(await screen.findByRole('heading', { name: 'Today' })).toBeDefined()
  expect(router.state.location.href).toBe('/')
})

test('a signed-in user who opens the sign-in page goes where they were headed', async () => {
  fakeServer(signedIn())

  const router = renderAt('/login?redirect=%2Fsettings')

  expect(await screen.findByRole('heading', { name: 'Settings' })).toBeDefined()
  expect(router.state.location.pathname).toBe('/settings')
})

test.each([
  [problem(401), 'Wrong email or password.'],
  [problem(429, { 'Retry-After': '600' }), 'Too many failed sign-ins. Try again in 10 minutes.'],
  [problem(429, { 'Retry-After': '61' }), 'Too many failed sign-ins. Try again in 2 minutes.'],
  [problem(429, { 'Retry-After': '60' }), 'Too many failed sign-ins. Try again in 1 minute.'],
  [problem(503, { 'Retry-After': '5' }), 'The server is busy. Try again in 5 seconds.'],
  [problem(503, { 'Retry-After': '1' }), 'The server is busy. Try again in 1 second.'],
  [problem(429), 'Signing in failed. Try again.'],
  [problem(500), 'Signing in failed. Try again.'],
])('a refused sign-in says why: %#', async (answer, message) => {
  server(() => answer)
  const router = renderAt('/')

  await signIn()

  expect((await screen.findByRole('alert')).textContent).toBe(message)
  expect(router.state.location.pathname).toBe('/login')
  expect(screen.getByRole('button', { name: 'Sign in' })).toHaveProperty('disabled', false)
})

test('a sign-in the server never got says the server cannot be reached', async () => {
  server(() => Promise.reject(new TypeError('Failed to fetch')))
  renderAt('/')

  await signIn()

  expect((await screen.findByRole('alert')).textContent).toBe('The server cannot be reached.')
})

test('the wait is said in the language of the app', async () => {
  setLocale('hr')
  server(() => problem(429, { 'Retry-After': '120' }))
  renderAt('/')

  fireEvent.change(await screen.findByLabelText('Email'), { target: { value: 'a@example.com' } })
  fireEvent.change(screen.getByLabelText('Lozinka'), { target: { value: 'x' } })
  fireEvent.click(screen.getByRole('button', { name: 'Prijavi se' }))

  expect((await screen.findByRole('alert')).textContent).toBe(
    'Previše neuspjelih prijava. Pokušaj ponovno za 2 minute.',
  )
})

test('a server with only a provider offers no password', async () => {
  fakeServer({
    ...signedOut,
    'GET /api/v1/auth/methods': () =>
      Response.json({ password: false, oidc: { name: 'Authentik', login_url: '/oidc' } }),
  })

  renderAt('/')

  expect(await screen.findByRole('link', { name: 'Sign in with Authentik' })).toBeDefined()
  expect(screen.queryByLabelText('Password')).toBeNull()
  expect(screen.queryByText('or')).toBeNull()
})

test('a server with no way to sign in says so', async () => {
  fakeServer({ ...signedOut, 'GET /api/v1/auth/methods': () => Response.json({ password: false }) })

  renderAt('/')

  expect(
    await screen.findByText('This server offers no way to sign in. Ask its admin.'),
  ).toBeDefined()
  expect(screen.queryByRole('link')).toBeNull()
})

test('the sign-in page says when the server cannot be reached', async () => {
  fakeServer({
    'GET /api/v1/auth/session': () => Promise.reject(new TypeError('Failed to fetch')),
    'GET /api/v1/auth/methods': () => Promise.reject(new TypeError('Failed to fetch')),
  })

  renderAt('/login')

  expect((await screen.findByRole('alert')).textContent).toBe('The server cannot be reached.')
})

test('signing out ends the session and forgets it, so the next visit signs in again', async () => {
  let live = true
  const fetch = fakeServer({
    ...signedOut,
    'GET /api/v1/auth/session': () => (live ? Response.json(session()) : problem(401)),
    'GET /api/v1/info': () => Response.json({ name: 'tdeemon', version: 'v1' }),
    'POST /api/v1/auth/logout': () => {
      live = false
      return new Response(null, { status: 204 })
    },
  })
  const router = renderAt('/settings')
  expect(await screen.findByText('Signed in as Ana (ana@example.com).')).toBeDefined()

  fireEvent.click(screen.getByRole('button', { name: 'Sign out' }))

  // The sign-in page asks the server again: a session kept in the app
  // would send the browser back to the sections.
  expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeDefined()
  expect(router.state.location.pathname).toBe('/login')
  expect(
    fetch.mock.calls.some(
      ([r]) => r instanceof Request && r.method === 'POST' && r.url.endsWith('/auth/logout'),
    ),
  ).toBe(true)
})

test('a sign-out that fails says so and stays', async () => {
  fakeServer({
    ...signedIn(),
    'POST /api/v1/auth/logout': () => problem(500),
  })
  const router = renderAt('/settings')

  fireEvent.click(await screen.findByRole('button', { name: 'Sign out' }))

  expect((await screen.findByRole('alert')).textContent).toBe('Signing out failed. Try again.')
  expect(router.state.location.pathname).toBe('/settings')
})

test('a section says when the server cannot say who is signed in, and tries again', async () => {
  let up = false
  fakeServer({
    'GET /api/v1/auth/session': () =>
      up ? Response.json(session()) : Promise.reject(new TypeError('Failed to fetch')),
  })
  renderAt('/settings')

  expect((await screen.findByRole('alert')).textContent).toBe('The server cannot be reached.')

  up = true
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }))

  expect(await screen.findByRole('heading', { name: 'Settings' })).toBeDefined()
})

test('a section says when the server failed to say who is signed in', async () => {
  fakeServer({ 'GET /api/v1/auth/session': () => problem(500) })

  renderAt('/charts')

  expect((await screen.findByRole('alert')).textContent).toBe('The server failed to answer.')
  await waitFor(() => {
    expect(screen.queryByRole('navigation')).toBeNull()
  })
})

test('signing in forgets what the app kept of whoever was signed in before', async () => {
  server(() => Response.json(session()))
  const router = renderAt('/login')
  const { queryClient } = router.options.context
  queryClient.setQueryData(['get', '/api/v1/days/{date}'], { meals: ['of another user'] })

  await signIn()

  expect(await screen.findByRole('heading', { name: 'Today' })).toBeDefined()
  expect(queryClient.getQueryData(['get', '/api/v1/days/{date}'])).toBeUndefined()
})

test('a 401 without a body, as a proxy may send, still asks to sign in', async () => {
  fakeServer({
    ...signedOut,
    'GET /api/v1/auth/session': () => new Response(null, { status: 401 }),
  })

  const router = renderAt('/foods')

  expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeDefined()
  expect(router.state.location.pathname).toBe('/login')
})

test('a target the server would refuse is the start page, not an error', async () => {
  fakeServer(signedIn())

  const router = renderAt('/login?redirect=%2F%09%2Fevil.example')

  expect(await screen.findByRole('heading', { name: 'Today' })).toBeDefined()
  expect(router.state.location.href).toBe('/')
})

test('the sign-in page says it is connecting until the server answers', async () => {
  fakeServer({
    ...signedOut,
    'GET /api/v1/auth/methods': () => new Promise<Response>(() => undefined),
  })

  renderAt('/login')

  expect(await screen.findByText('Connecting to the server…')).toBeDefined()
  expect(screen.queryByLabelText('Password')).toBeNull()
})

test('a failure of the app itself is not blamed on the server', async () => {
  // A session without its user: reading its locale is a TypeError, as most
  // bugs are, and not a request that got no answer.
  fakeServer({
    'GET /api/v1/auth/session': () => Response.json({ expires_at: '2026-11-22T10:00:00Z' }),
  })

  renderAt('/charts')

  expect((await screen.findByRole('alert')).textContent).toBe('Something went wrong.')
  // Within the app's frame, with the way to try again.
  expect(screen.getByRole('main')).toBeDefined()
  expect(screen.getByRole('button', { name: 'Try again' })).toBeDefined()
})
