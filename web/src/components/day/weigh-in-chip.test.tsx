import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { day, fakeServer, renderAt, signedIn, today } from '@/test/app'
import { dayServer } from '@/test/day-server'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

test("the quick add's Weigh-in chip takes a weigh-in in place, and the meal stays as it was", async () => {
  const server = dayServer()
  fakeServer({
    ...signedIn(),
    ...server.routes,
    'GET /api/v1/days/2026-10-22': () =>
      Response.json(
        day({
          date: '2026-10-22',
          metrics: [{ kind: 'weight', value: 80.4, unit: 'kg', source: 'manual' }],
        }),
      ),
  })
  renderAt('/?log=true')

  const sheet = within(await screen.findByRole('dialog', { name: 'A new meal' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Weigh-in' }))
  const field = sheet.getByLabelText('Weight')
  expect(document.activeElement).toBe(field)
  await waitFor(() => {
    expect(field.getAttribute('placeholder')).toBe('80.4')
  })
  fireEvent.change(field, { target: { value: '80.1' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Save the weigh-in' }))

  // Said over the sheet, with Undo; the chip is back, with the focus.
  expect(await screen.findByText('Saved: Weight')).toBeDefined()
  expect(server.sent).toEqual([
    { method: 'PUT', path: `/api/v1/days/${today}/metrics/weight`, body: { value: 80.1 } },
  ])
  await waitFor(() => {
    expect(document.activeElement).toBe(sheet.getByRole('button', { name: 'Weigh-in' }))
  })
  expect(screen.getByRole('dialog', { name: 'A new meal' })).toBeDefined()
  expect(sheet.getByRole('button', { name: 'Log the meal' })).toBeDefined()
})
