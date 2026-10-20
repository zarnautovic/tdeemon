import { QueryClient } from '@tanstack/react-query'
import { createMemoryHistory } from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'
import { App } from '@/app'
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
})

test("the start page shows the server's name and version from the API", async () => {
  // A name unlike the page heading, so the test sees the API's value.
  const fetch = vi.fn<typeof globalThis.fetch>(() =>
    Promise.resolve(Response.json({ name: 'tdeemon-test', version: 'v1.2.3' })),
  )
  vi.stubGlobal('fetch', fetch)

  renderAt('/')

  expect((await screen.findByTestId('server-version')).textContent).toBe('v1.2.3')
  expect(screen.getByTestId('server-name').textContent).toBe('tdeemon-test')
  expect(fetch).toHaveBeenCalledOnce()
  const request = fetch.mock.calls[0]?.[0]
  expect(request instanceof Request ? new URL(request.url).pathname : request).toBe('/api/v1/info')
})

test('the start page says so when the server cannot be reached', async () => {
  vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))))

  renderAt('/')

  expect(await screen.findByText('The server cannot be reached.')).toBeDefined()
})

test('an unknown path shows the not-found page with a way back', async () => {
  vi.stubGlobal('fetch', vi.fn())

  renderAt('/no/such/page')

  expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeDefined()
  expect(screen.getByRole('link', { name: 'Back to the start' }).getAttribute('href')).toBe('/')
})
