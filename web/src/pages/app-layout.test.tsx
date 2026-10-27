import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { setTheme } from '@/lib/theme'
import { day, fakeServer, renderAt, signedIn, today } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  setTheme('system')
  localStorage.clear()
})

function nav() {
  return screen.findByRole('navigation', { name: 'Sections' })
}

test('the sections are linked from one navigation, the current one marked', async () => {
  fakeServer(signedIn())
  renderAt('/foods')

  const links = within(await nav()).getAllByRole('link')
  // A phone shows the bar's (Log a meal its middle), a desktop the
  // sidebar's (Log a meal first, the settings at the foot): each hides
  // the other's.
  expect(links.map((a) => [a.textContent, a.getAttribute('href')])).toEqual([
    ['Log a mealN', '/?log=true'],
    ['Today', '/'],
    ['Foods', '/foods'],
    ['', '/?log=true'],
    ['Progress', '/progress'],
    ['Plan', '/plan'],
    ['Settings', '/settings'],
  ])
  expect(links.filter((a) => a.getAttribute('aria-current') === 'page')).toEqual([links[2]])
  expect(within(await nav()).getAllByRole('link', { name: 'Log a meal' })).toHaveLength(2)
})

test('the charts are the progress now, and the plan its own section', async () => {
  fakeServer(signedIn())

  const charts = renderAt('/charts?days=30')
  await waitFor(() => {
    expect(charts.state.location.pathname).toBe('/progress')
  })
  expect(charts.state.location.search).toEqual({ days: 30 })
  cleanup()

  const plan = renderAt('/settings/plan')
  await waitFor(() => {
    expect(plan.state.location.pathname).toBe('/plan')
  })
})

test('the user is named in the sidebar, by their initials too', async () => {
  fakeServer(signedIn())
  renderAt('/')

  const user = await within(await nav()).findByTestId('user')
  expect(user.textContent).toBe('AAna')
})

test("the sidebar's dark theme is a switch of the theme shown", async () => {
  fakeServer(signedIn())
  renderAt('/')

  // The system's is light here (jsdom has no matchMedia).
  const dark = await within(await nav()).findByRole('button', { name: 'Dark theme' })
  expect(dark.getAttribute('aria-pressed')).toBe('false')
  fireEvent.click(dark)
  expect(document.documentElement.classList.contains('dark')).toBe(true)
  expect(dark.getAttribute('aria-pressed')).toBe('true')
  fireEvent.click(dark)
  expect(document.documentElement.classList.contains('light')).toBe(true)
  expect(dark.getAttribute('aria-pressed')).toBe('false')
})

test("the system's dark theme is the one shown, and changes with it", async () => {
  let dark = true
  const listeners = new Set<() => void>()
  vi.stubGlobal('matchMedia', (query: string) => ({
    get matches() {
      return query === '(prefers-color-scheme: dark)' && dark
    },
    addEventListener: (_: string, f: () => void) => listeners.add(f),
    removeEventListener: (_: string, f: () => void) => listeners.delete(f),
  }))
  fakeServer(signedIn())
  renderAt('/')

  const button = await within(await nav()).findByRole('button', { name: 'Dark theme' })
  expect(button.getAttribute('aria-pressed')).toBe('true')
  act(() => {
    dark = false
    for (const f of listeners) {
      f()
    }
  })
  expect(button.getAttribute('aria-pressed')).toBe('false')
})

test('Log a meal opens the form on today, from another section', async () => {
  fakeServer(signedIn())
  const router = renderAt('/foods')

  // The bar's, a phone's.
  const [, bar] = within(await nav()).getAllByRole('link', { name: 'Log a meal' })
  if (bar === undefined) {
    throw new Error('no Log a meal in the bar')
  }
  fireEvent.click(bar)

  expect(await screen.findByRole('form', { name: 'A new meal' })).toBeDefined()
  // Once open, the address is today's again: a reload does not open it.
  await waitFor(() => {
    expect(router.state.location.search).toEqual({})
  })
  expect(router.state.location.pathname).toBe('/')
})

test('N logs a meal, / finds a food, and ? lists the shortcuts', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods': () => Response.json({ foods: [] }),
  })
  const router = renderAt('/foods')
  await nav()

  fireEvent.keyDown(window, { key: 'n' })
  expect(await screen.findByRole('form', { name: 'A new meal' })).toBeDefined()
  expect(router.state.location.pathname).toBe('/')

  fireEvent.keyDown(document.body, { key: '/' })
  const search = await screen.findByRole('searchbox', { name: 'Search the foods' })
  await waitFor(() => {
    expect(document.activeElement).toBe(search)
  })

  fireEvent.keyDown(document.body, { key: '?' })
  const dialog = await screen.findByRole('dialog', { name: 'Keyboard shortcuts' })
  expect(within(dialog).getByText('Log a meal')).toBeDefined()
  expect(within(dialog).getByText('The day before or after')).toBeDefined()
})

test('a shortcut is not taken from a field, nor with a modifier', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods': () => Response.json({ foods: [] }),
  })
  const router = renderAt('/foods')
  const search = await screen.findByRole('searchbox', { name: 'Search the foods' })

  fireEvent.keyDown(search, { key: 'n' })
  fireEvent.keyDown(document.body, { key: 'n', ctrlKey: true })
  fireEvent.keyDown(document.body, { key: 'n', metaKey: true })

  await new Promise((resolve) => setTimeout(resolve, 50))
  expect(router.state.location.pathname).toBe('/foods')
})

test('the arrows move a day back and ahead, up to today', async () => {
  const yesterday = '2026-10-22'
  fakeServer({
    ...signedIn(),
    [`GET /api/v1/days/${yesterday}`]: () => Response.json(day({ date: yesterday })),
  })
  const router = renderAt('/')
  await screen.findByRole('heading', { name: 'Today' })

  fireEvent.keyDown(document.body, { key: 'ArrowRight' })
  fireEvent.keyDown(document.body, { key: 'ArrowLeft' })
  await waitFor(() => {
    expect(router.state.location.search).toEqual({ date: yesterday })
  })

  fireEvent.keyDown(document.body, { key: 'ArrowRight' })
  await waitFor(() => {
    expect(router.state.location.search).toEqual({})
  })
  expect(today).toBe('2026-10-23')
})
