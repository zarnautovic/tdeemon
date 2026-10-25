import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { day, fakeServer, problem, renderAt, session, signedIn, signedOut, today } from '@/test/app'
import { dayServer } from '@/test/day-server'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

const path = `/api/v1/days/${today}`

// formOf is the form a field is in.
function formOf(field: HTMLElement): HTMLElement {
  const form = field.closest('form')
  if (!form) {
    throw new Error('the field is in no form')
  }
  return form
}

function measurements() {
  return within(screen.getByRole('region', { name: 'Measurements' }))
}

test('a weighing is typed in and saved, and the field gives way to it', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  fireEvent.change(await screen.findByLabelText('Weight'), { target: { value: '72.5' } })
  fireEvent.click(measurements().getByRole('button', { name: 'Save' }))

  expect(await measurements().findByText('72.5 kg')).toBeDefined()
  expect(server.sent).toEqual([
    { method: 'PUT', path: `${path}/metrics/weight`, body: { value: 72.5 } },
  ])
  expect(screen.queryByLabelText('Weight')).toBeNull()
})

test('a weighing typed in Croatian has its decimal comma', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn('hr'), ...server.routes })
  renderAt('/')

  fireEvent.change(await screen.findByLabelText('Kilaža'), { target: { value: '72,5' } })
  fireEvent.click(screen.getByRole('button', { name: 'Spremi' }))

  expect(await screen.findByText('72,5 kg')).toBeDefined()
  expect(server.sent.at(-1)?.body).toEqual({ value: 72.5 })
})

test('what is no number is not sent, and the field says so', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  fireEvent.change(await screen.findByLabelText('Weight'), { target: { value: '72.5 kg' } })
  fireEvent.click(measurements().getByRole('button', { name: 'Save' }))

  expect(measurements().getByRole('alert').textContent).toBe('Write a number, such as 72.5.')
  expect(screen.getByLabelText('Weight').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])
})

test("a value out of the kind's bounds is said with them, in the language, and not sent", async () => {
  const server = dayServer()
  fakeServer({ ...signedIn('hr'), ...server.routes })
  renderAt('/')

  fireEvent.change(await screen.findByLabelText('Kilaža'), { target: { value: '7,25' } })
  const mjerenja = within(screen.getByRole('region', { name: 'Mjerenja' }))
  fireEvent.click(mjerenja.getByRole('button', { name: 'Spremi' }))

  expect(mjerenja.getByRole('alert').textContent).toBe('Upiši vrijednost od 20 do 400 kg.')
  expect(screen.getByLabelText('Kilaža').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent).toEqual([])
})

test('a value the server refuses says so, with nothing to try again', async () => {
  fakeServer({
    ...signedIn(),
    ...dayServer().routes,
    [`PUT ${path}/metrics/weight`]: () => problem(422),
  })
  renderAt('/')

  fireEvent.change(await screen.findByLabelText('Weight'), { target: { value: '72.5' } })
  fireEvent.click(measurements().getByRole('button', { name: 'Save' }))

  expect((await measurements().findByRole('alert')).textContent).toBe(
    'The server does not take this value.',
  )
  expect(measurements().queryByRole('button', { name: 'Try again' })).toBeNull()
})

test('a weighing that did not reach the server is sent again', async () => {
  const server = dayServer()
  let up = false
  fakeServer({
    ...signedIn(),
    ...server.routes,
    [`PUT ${path}/metrics/weight`]: (request) =>
      up
        ? (server.routes[`PUT ${path}/metrics/weight`]?.(request) ?? Response.error())
        : Promise.reject(new TypeError('Failed to fetch')),
  })
  renderAt('/')

  fireEvent.change(await screen.findByLabelText('Weight'), { target: { value: '72.5' } })
  fireEvent.click(measurements().getByRole('button', { name: 'Save' }))
  expect((await measurements().findByRole('alert')).textContent).toContain(
    'Not saved. The server cannot be reached.',
  )

  up = true
  fireEvent.click(measurements().getByRole('button', { name: 'Try again' }))

  expect(await measurements().findByText('72.5 kg')).toBeDefined()
})

test('a measurement is corrected, starting from its value', async () => {
  const server = dayServer(
    day({ metrics: [{ kind: 'steps', value: 8042, unit: 'steps', source: 'import' }] }),
  )
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Change: Steps' }))
  const steps = screen.getByLabelText('Steps')
  expect(steps).toHaveProperty('value', '8042')
  fireEvent.change(steps, { target: { value: '9,100' } })
  fireEvent.click(within(formOf(steps)).getByRole('button', { name: 'Save' }))

  expect(await measurements().findByText('9,100')).toBeDefined()
  expect(server.sent).toEqual([
    { method: 'PUT', path: `${path}/metrics/steps`, body: { value: 9100 } },
  ])
  expect(screen.queryByLabelText('Steps')).toBeNull()
})

test('a correction can be given up', async () => {
  const server = dayServer(
    day({ metrics: [{ kind: 'sleep_h', value: 7.25, unit: 'h', source: 'import' }] }),
  )
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Change: Sleep' }))
  fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

  expect(measurements().getByText('7.3 h')).toBeDefined()
  expect(server.sent).toEqual([])
})

test('a wrong measurement is deleted', async () => {
  const server = dayServer(
    day({ metrics: [{ kind: 'weight', value: 27.5, unit: 'kg', source: 'manual' }] }),
  )
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Change: Weight' }))
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

  expect(await measurements().findByText('None yet.')).toBeDefined()
  expect(server.sent).toEqual([
    { method: 'DELETE', path: `${path}/metrics/weight`, body: undefined },
  ])
  // The day has no weighing again: its field is back.
  expect(screen.getByLabelText('Weight')).toBeDefined()
})

test('a delete that failed says so', async () => {
  fakeServer({
    ...signedIn(),
    ...dayServer(day({ metrics: [{ kind: 'hrv', value: 52, unit: 'ms', source: 'import' }] }))
      .routes,
    [`DELETE ${path}/metrics/hrv`]: () => problem(500),
  })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Change: Heart rate variability' }))
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

  expect((await measurements().findByRole('alert')).textContent).toContain(
    'Not saved. The server failed to answer.',
  )
})

test('another kind of measurement is added, of those the day has none of', async () => {
  const server = dayServer(
    day({ metrics: [{ kind: 'steps', value: 8042, unit: 'steps', source: 'import' }] }),
  )
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Add a measurement' }))
  const kind = screen.getByLabelText('Measurement')
  const options = within(kind)
    .getAllByRole('option')
    .map((o) => o.textContent)
  expect(options).not.toContain('Weight')
  expect(options).not.toContain('Steps')
  expect(options[0]).toBe('Body fat')
  fireEvent.change(kind, { target: { value: 'sleep_h' } })
  fireEvent.change(screen.getByLabelText('Value'), { target: { value: '7.5' } })
  fireEvent.click(
    within(formOf(screen.getByLabelText('Value'))).getByRole('button', {
      name: 'Save',
    }),
  )

  expect(await measurements().findByText('7.5 h')).toBeDefined()
  expect(server.sent).toEqual([
    { method: 'PUT', path: `${path}/metrics/sleep_h`, body: { value: 7.5 } },
  ])
  expect(screen.queryByLabelText('Measurement')).toBeNull()
})

test('a change refused for want of a session sends the browser to sign in', async () => {
  let live = true
  fakeServer({
    ...signedIn(),
    ...signedOut,
    'GET /api/v1/auth/session': () => (live ? Response.json(session()) : problem(401)),
    [`PUT ${path}/metrics/weight`]: () => {
      live = false
      return problem(401)
    },
  })
  const router = renderAt('/')

  fireEvent.change(await screen.findByLabelText('Weight'), { target: { value: '72.5' } })
  fireEvent.click(measurements().getByRole('button', { name: 'Save' }))

  expect(await screen.findByRole('heading', { name: 'Sign in' })).toBeDefined()
  await waitFor(() => {
    expect(router.state.location.pathname).toBe('/login')
  })
})

test.each([
  ['en', 'Change: Resting energy', 'Resting energy', '1834.27'],
  ['hr', 'Promijeni: Energija u mirovanju', 'Energija u mirovanju', '1834,27'],
] as const)(
  'a value of thousands with decimals is corrected as it is, in %s',
  async (locale, change, label, initial) => {
    const server = dayServer(
      day({ metrics: [{ kind: 'basal_kcal', value: 1834.27, unit: 'kcal', source: 'import' }] }),
    )
    fakeServer({ ...signedIn(locale), ...server.routes })
    renderAt('/')

    fireEvent.click(await screen.findByRole('button', { name: change }))
    const field = screen.getByLabelText(label)
    expect(field).toHaveProperty('value', initial)
    fireEvent.click(
      within(formOf(field)).getByRole('button', { name: locale === 'en' ? 'Save' : 'Spremi' }),
    )

    await waitFor(() => {
      expect(server.sent.map((s) => s.body)).toEqual([{ value: 1834.27 }])
    })
    expect(server.day().metrics[0]?.value).toBe(1834.27)
    await waitFor(() => {
      expect(screen.queryByLabelText(label)).toBeNull()
    })
  },
)

test("a field's unit is said with it", async () => {
  fakeServer({ ...signedIn(), ...dayServer().routes })
  renderAt('/')

  const weight = await screen.findByLabelText('Weight')
  const unit = document.getElementById(weight.getAttribute('aria-describedby') ?? '')
  expect(unit?.textContent).toBe('kg')
})

test('a measurement of a day to come is not offered to correct', async () => {
  fakeServer({
    ...signedIn(),
    ...dayServer(
      day({
        date: '2026-10-25',
        metrics: [{ kind: 'steps', value: 120, unit: 'steps', source: 'import' }],
      }),
    ).routes,
  })
  renderAt('/?date=2026-10-25')

  expect(await screen.findByText('120')).toBeDefined()
  expect(measurements().queryByRole('button', { name: 'Change: Steps' })).toBeNull()
})
