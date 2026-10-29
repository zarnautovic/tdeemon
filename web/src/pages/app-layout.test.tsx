import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { sessionQuery } from '@/api/session'
import { setLocale } from '@/i18n/locale'
import { setTheme } from '@/lib/theme'
import { day, fakeServer, renderAt, signedIn, today, unreachable } from '@/test/app'

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
  // A phone shows the bar's, a desktop the sidebar's (the settings at the
  // foot): each hides the other's. The sidebar's sections show their keys,
  // G and a letter, as the pointer or the focus is on them; their names
  // stay their own.
  expect(links.map((a) => [a.textContent, a.getAttribute('href')])).toEqual([
    ['TodayG D', '/'],
    ['FoodsG F', '/foods'],
    ['ProgressG P', '/progress'],
    ['PlanG L', '/plan'],
    ['SettingsG S', '/settings'],
  ])
  expect(links.filter((a) => a.getAttribute('aria-current') === 'page')).toEqual([links[1]])
  // Log a meal is no section, but a button of each: the bar's middle, the
  // sidebar's first.
  expect(within(await nav()).getAllByRole('button', { name: 'Log a meal' })).toHaveLength(2)
  expect(within(await nav()).getByRole('link', { name: 'Today' })).toBe(links[0])
})

// The keyboard's first stop skips the sections to the page, which takes
// the focus, the address as it was.
test('the first stop of the keyboard skips to the content', async () => {
  fakeServer(signedIn())
  const router = renderAt('/foods')
  await nav()

  const first = document.querySelector<HTMLElement>('a[href], button, input, [tabindex="0"]')
  expect(first?.textContent).toBe('Skip to content')
  if (first === null) {
    throw new Error('nothing to stop at')
  }
  fireEvent.click(first)

  expect(document.activeElement).toBe(screen.getByRole('main'))
  expect(router.state.location.href).toBe('/foods')
})

test('G, then a letter, goes to a section; the letter alone does not', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods': () => Response.json({ foods: [] }),
  })
  const router = renderAt('/')
  await screen.findByRole('heading', { name: 'Today' })

  fireEvent.keyDown(document.body, { key: 'f' })
  await new Promise((resolve) => setTimeout(resolve, 20))
  expect(router.state.location.pathname).toBe('/')

  // goes presses G and letter, and waits to be at path.
  const goes = async (letter: string, path: string) => {
    fireEvent.keyDown(document.body, { key: 'g' })
    fireEvent.keyDown(document.body, { key: letter })
    await waitFor(() => {
      expect(router.state.location.pathname).toBe(path)
    })
  }
  await goes('f', '/foods')
  await goes('p', '/progress')
  await goes('l', '/plan')
  await goes('s', '/settings')
  await goes('d', '/')
})

test('⌘Z or Ctrl+Z takes back the latest change, and the toast says which', async () => {
  const undos: unknown[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods': () => Response.json({ foods: [] }),
    'POST /api/v1/undo': async (request) => {
      undos.push(await request.json())
      return Response.json({
        undone: {
          action: 'meal.delete',
          entity: '0193c3a5-0000-7000-8000-0000000000a1',
          at: `${today}T16:00:00Z`,
          client: 'mcp',
          days: [today],
        },
      })
    },
  })
  renderAt('/foods')
  const search = await screen.findByRole('searchbox', { name: 'Search the foods' })

  // A field's own undo is the field's.
  fireEvent.keyDown(search, { key: 'z', ctrlKey: true })
  fireEvent.keyDown(document.body, { key: 'z', ctrlKey: true })

  expect(await screen.findByText('Taken back: a meal deleted.')).toBeDefined()
  // The latest, whichever it is: no change named, an id of its own. The
  // day it was of is linked, as the foods are shown.
  expect(undos).toEqual([{ id: expect.any(String) as unknown }])
  expect(screen.getByRole('link', { name: 'See it today' })).toBeDefined()

  fireEvent.keyDown(document.body, { key: 'z', metaKey: true })
  await waitFor(() => {
    expect(undos).toHaveLength(2)
  })
  // Ctrl+Shift+Z is another key: a redo, which the app has none of.
  fireEvent.keyDown(document.body, { key: 'Z', ctrlKey: true, shiftKey: true })
  await new Promise((resolve) => setTimeout(resolve, 20))
  expect(undos).toHaveLength(2)
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

test('Log a meal opens the form on today over the section, which stays', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods/list': () => Response.json({ foods: [], total: 0 }),
  })
  const router = renderAt('/foods?filter=archived')

  // The bar's, a phone's.
  const [, bar] = within(await nav()).getAllByRole('button', { name: 'Log a meal' })
  if (bar === undefined) {
    throw new Error('no Log a meal in the bar')
  }
  fireEvent.click(bar)

  const sheet = await screen.findByRole('dialog', { name: 'A new meal' })
  expect(router.state.location.pathname).toBe('/foods')
  expect(router.state.location.search).toEqual({ filter: 'archived' })

  // Closed, the focus is back on the bar's Log a meal.
  fireEvent.click(within(sheet).getByRole('button', { name: 'Close' }))
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'A new meal' })).toBeNull()
  })
  expect(document.activeElement).toBe(bar)
})

test('N logs a meal, / finds a food, and ? lists the shortcuts', async () => {
  fakeServer({
    ...signedIn(),
    'GET /api/v1/foods': () => Response.json({ foods: [] }),
  })
  const router = renderAt('/foods')
  await nav()

  fireEvent.keyDown(window, { key: 'n' })
  const sheet = await screen.findByRole('dialog', { name: 'A new meal' })
  expect(router.state.location.pathname).toBe('/foods')
  fireEvent.click(within(sheet).getByRole('button', { name: 'Close' }))
  await waitFor(() => {
    expect(screen.queryByRole('dialog', { name: 'A new meal' })).toBeNull()
  })

  fireEvent.keyDown(document.body, { key: '/' })
  const search = await screen.findByRole('searchbox', { name: 'Search the foods' })
  await waitFor(() => {
    expect(document.activeElement).toBe(search)
  })

  fireEvent.keyDown(document.body, { key: '?' })
  const dialog = await screen.findByRole('dialog', { name: 'Keyboard shortcuts' })
  expect(within(dialog).getByText('Log a meal')).toBeDefined()
  expect(within(dialog).getByText('The day before or after')).toBeDefined()
  // The toasts' key, which reaches them from a sheet too.
  expect(within(dialog).getByText('The notifications')).toBeDefined()
  expect(within(dialog).getByText('F6')).toBeDefined()
  // Every other: the undo, the sections and a meal's.
  const said = (what: string) => within(dialog).getByText(what).nextElementSibling?.textContent
  expect(said('Undo the latest change')).toBe('Ctrl+Z')
  expect(said('Foods')).toBe('GthenF')
  expect(said('Settings')).toBe('GthenS')
  expect(said('The meal before or after')).toBe('↑↓')
  expect(said('Change the meal')).toBe('Enter')
  expect(said('Repeat the meal today')).toBe('R')
  expect(said('Delete the meal')).toBe('Delete')
})

test('a section opens in the frame on the session the app has, when the server cannot say it', async () => {
  fakeServer(signedIn())
  const router = renderAt('/')
  await screen.findByRole('region', { name: 'Meals' })
  // The session the app has is old enough to be asked again on the way.
  await act(() =>
    router.options.context.queryClient.invalidateQueries({
      queryKey: sessionQuery.queryKey,
      refetchType: 'none',
    }),
  )

  unreachable()
  fireEvent.click(within(await nav()).getByRole('link', { name: 'Foods' }))

  const alert = await screen.findByRole('alert')
  expect(within(alert).getByText("Couldn't load your foods.")).toBeDefined()
  expect(screen.getByRole('heading', { level: 1, name: 'Foods' })).toBeDefined()
  expect(screen.getByRole('navigation', { name: 'Sections' })).toBeDefined()
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
  await screen.findByRole('link', { name: 'The day before' })

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
