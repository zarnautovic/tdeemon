import { QueryClient } from '@tanstack/react-query'
import { createMemoryHistory } from '@tanstack/react-router'
import { act, cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { App } from '@/app'
import { setLocale } from '@/i18n/locale'
import { createAppRouter } from '@/router'

function renderAt(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createAppRouter({
    queryClient,
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  render(<App router={router} queryClient={queryClient} />)
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

test("the settings show the server's name and version from the API", async () => {
  // A name unlike the page's text, so the test sees the API's value.
  const fetch = vi.fn<typeof globalThis.fetch>(() =>
    Promise.resolve(Response.json({ name: 'tdeemon-test', version: 'v1.2.3' })),
  )
  vi.stubGlobal('fetch', fetch)

  renderAt('/settings')

  expect((await screen.findByTestId('server-version')).textContent).toBe('v1.2.3')
  expect(screen.getByTestId('server-name').textContent).toBe('tdeemon-test')
  expect(fetch).toHaveBeenCalledOnce()
  const request = fetch.mock.calls[0]?.[0]
  expect(request instanceof Request ? new URL(request.url).pathname : request).toBe('/api/v1/info')
})

test('the settings say so when the server cannot be reached', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))),
  )

  renderAt('/settings')

  expect(await screen.findByText('The server cannot be reached.')).toBeDefined()
})

test('every section links the others, and marks itself as the current one', async () => {
  vi.stubGlobal('fetch', vi.fn())

  renderAt('/foods')

  const nav = await screen.findByRole('navigation', { name: 'Sections' })
  const links = within(nav).getAllByRole('link')
  expect(links.map((a) => [a.textContent, a.getAttribute('href')])).toEqual([
    ['Day', '/'],
    ['Foods', '/foods'],
    ['Charts', '/charts'],
    ['Settings', '/settings'],
  ])
  expect(links.filter((a) => a.getAttribute('aria-current') === 'page')).toEqual([links[1]])
  expect(screen.getByRole('heading', { name: 'Foods' })).toBeDefined()
})

test("the day is the start page, and only it is current there, not every path under '/'", async () => {
  vi.stubGlobal('fetch', vi.fn())

  renderAt('/')

  const nav = await screen.findByRole('navigation', { name: 'Sections' })
  const current = within(nav)
    .getAllByRole('link')
    .filter((a) => a.getAttribute('aria-current') === 'page')
  expect(current.map((a) => a.textContent)).toEqual(['Day'])
  expect(screen.getByRole('heading', { name: 'Today' })).toBeDefined()
})

test('an unknown path shows the not-found page with a way back, without the sections', async () => {
  vi.stubGlobal('fetch', vi.fn())

  renderAt('/no/such/page')

  expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeDefined()
  expect(screen.getByRole('main')).toBeDefined()
  expect(screen.getByRole('link', { name: 'Back to the start' }).getAttribute('href')).toBe('/')
  expect(screen.queryByRole('navigation')).toBeNull()
})

test('the app speaks the language set, and the document says which', async () => {
  vi.stubGlobal('fetch', vi.fn())
  renderAt('/charts')
  expect(await screen.findByRole('heading', { name: 'Charts' })).toBeDefined()
  expect(document.documentElement.lang).toBe('en')

  act(() => {
    setLocale('hr')
  })

  expect(await screen.findByRole('heading', { name: 'Grafovi' })).toBeDefined()
  const nav = screen.getByRole('navigation', { name: 'Odjeljci' })
  expect(within(nav).getByRole('link', { name: 'Postavke' })).toBeDefined()
  expect(document.documentElement.lang).toBe('hr')
})
