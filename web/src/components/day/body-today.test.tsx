import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { day, fakeServer, renderAt, signedIn, today } from '@/test/app'
import { dayServer } from '@/test/day-server'
import { newUser, summaryOfNothing } from '@/test/new-user'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

const path = `/api/v1/days/${today}`

// yesterday is the day before today, weighed as given.
function yesterday(source: 'manual' | 'import' = 'manual') {
  return {
    'GET /api/v1/days/2026-10-22': () =>
      Response.json(
        day({
          date: '2026-10-22',
          metrics: [{ kind: 'weight', value: 82.5, unit: 'kg', source }],
        }),
      ),
  }
}

function body() {
  return within(screen.getByRole('region', { name: 'Body today' }))
}

test("the weight's tile is a field in its place, hinting with the day before's, and saved", async () => {
  const server = dayServer(
    day({ metrics: [{ kind: 'sleep_h', value: 7.5, unit: 'h', source: 'import' }] }),
  )
  fakeServer({ ...signedIn(), ...server.routes, ...yesterday() })
  renderAt('/')

  await screen.findByRole('region', { name: 'Body today' })
  fireEvent.click(body().getByRole('button', { name: /^Weight/ }))
  const field = body().getByLabelText('Weight')
  expect(document.activeElement).toBe(field)
  expect(field.getAttribute('inputmode')).toBe('decimal')
  await waitFor(() => {
    expect(field.getAttribute('placeholder')).toBe('82.5')
  })
  fireEvent.change(field, { target: { value: '82.1' } })
  // Enter submits the field's form, as ✓ does.
  fireEvent.submit(field)

  expect(await screen.findByText('Saved: Weight')).toBeDefined()
  expect(screen.getByRole('button', { name: 'Undo' })).toBeDefined()
  expect(server.sent).toEqual([
    { method: 'PUT', path: `${path}/metrics/weight`, body: { value: 82.1 } },
  ])
  // The tile is back, with the weight, and the focus.
  const tile = await body().findByRole('button', { name: /^Weight82\.1 kg/ })
  expect(document.activeElement).toBe(tile)
})

test('a weight given up leaves nothing sent, and its tile the focus', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  await screen.findByRole('region', { name: 'Body today' })
  fireEvent.click(body().getByRole('button', { name: /^Weight/ }))
  fireEvent.keyDown(body().getByLabelText('Weight'), { key: 'Escape' })
  await waitFor(() => {
    expect(document.activeElement).toBe(body().getByRole('button', { name: /^Weight/ }))
  })
  expect(server.sent).toEqual([])
})

test('another tile opens the measurements at its own: one to correct, one to add', async () => {
  const server = dayServer(
    day({ metrics: [{ kind: 'sleep_h', value: 7.5, unit: 'h', source: 'import' }] }),
  )
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  await screen.findByRole('region', { name: 'Body today' })
  fireEvent.click(body().getByRole('button', { name: /^Sleep/ }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Measurements' }))
  const sleep = sheet.getByLabelText('Sleep')
  expect(sleep).toHaveProperty('value', '7.5')
  await waitFor(() => {
    expect(document.activeElement).toBe(sleep)
  })
  fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' })
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  fireEvent.click(body().getByRole('button', { name: /^Steps/ }))
  const again = within(await screen.findByRole('dialog', { name: 'Measurements' }))
  expect(again.getByLabelText('Measurement')).toHaveProperty('value', 'steps')
  const value = again.getByLabelText('Value')
  await waitFor(() => {
    expect(document.activeElement).toBe(value)
  })
  fireEvent.change(value, { target: { value: '9000' } })
  fireEvent.submit(value)
  await waitFor(() => {
    expect(server.sent).toEqual([
      { method: 'PUT', path: `${path}/metrics/steps`, body: { value: 9000 } },
    ])
  })
})

test('a day to come has tiles that open nothing', async () => {
  const date = '2026-10-25'
  fakeServer({
    ...signedIn(),
    [`GET /api/v1/days/${date}`]: () => Response.json(day({ date })),
  })
  renderAt(`/?date=${date}`)

  const tiles = within(await screen.findByRole('region', { name: 'Body' }))
  expect(tiles.getByText('Weight')).toBeDefined()
  expect(tiles.queryByRole('button', { name: /^Weight/ })).toBeNull()
})

test("a new user's body is only the way to a first weigh-in, no empty tiles", async () => {
  const server = dayServer(day({ metrics: [] }))
  fakeServer({ ...newUser(), ...server.routes })
  renderAt('/')

  const region = await screen.findByRole('region', { name: 'Body today' })
  const add = await within(region).findByRole('button', { name: 'Add a weigh-in' })
  expect(within(region).queryByText('—')).toBeNull()
  expect(within(region).queryByRole('button', { name: 'All measurements' })).toBeNull()

  fireEvent.click(add)
  const field = within(region).getByLabelText('Weight')
  expect(document.activeElement).toBe(field)
  fireEvent.change(field, { target: { value: '72.5' } })
  fireEvent.click(within(region).getByRole('button', { name: 'Save the weigh-in' }))

  // The first weigh-in, and the tiles with it.
  expect(await within(region).findByRole('button', { name: /^Weight72\.5 kg/ })).toBeDefined()
  expect(within(region).getByRole('button', { name: 'All measurements' })).toBeDefined()
})

test('a user who measured before has the tiles, though today has none', async () => {
  fakeServer({
    ...newUser(),
    'GET /api/v1/summary': () =>
      Response.json(summaryOfNothing({ trend: { date: '2026-10-20', kg: 80.1 } })),
  })
  renderAt('/')

  const region = within(await screen.findByRole('region', { name: 'Body today' }))
  expect(await region.findByRole('button', { name: /^Weight—trend 80\.1 kg/ })).toBeDefined()
  expect(region.queryByRole('button', { name: 'Add a weigh-in' })).toBeNull()
})

test('a weigh-in out of its bounds is said, and not sent', async () => {
  const server = dayServer()
  fakeServer({ ...signedIn(), ...server.routes })
  renderAt('/')

  await screen.findByRole('region', { name: 'Body today' })
  fireEvent.click(body().getByRole('button', { name: /^Weight/ }))
  const field = body().getByLabelText('Weight')
  fireEvent.change(field, { target: { value: '5' } })
  fireEvent.submit(field)
  expect((await body().findByRole('alert')).textContent).toBe('Give a value from 20 to 400 kg.')
  expect(field.getAttribute('aria-invalid')).toBe('true')
  fireEvent.change(field, { target: { value: 'abc' } })
  expect(body().queryByRole('alert')).toBeNull()
  fireEvent.submit(field)
  expect((await body().findByRole('alert')).textContent).toBe('Write a number, such as 72.5.')
  expect(server.sent).toEqual([])
})
