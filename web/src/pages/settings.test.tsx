import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { setLocale } from '@/i18n/locale'
import { fakeServer, me, problem, renderAt, session, signedIn } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

const phone = {
  id: '0193c3a5-0000-7000-8000-0000000000c1',
  name: 'iPhone',
  scopes: ['ingest' as const],
  created_at: '2026-10-01T08:00:00Z',
  last_used_at: '2026-10-23T06:00:00Z',
}
const claude = {
  id: '0193c3a5-0000-7000-8000-0000000000c2',
  name: 'Claude',
  scopes: ['read' as const, 'tools' as const],
  created_at: '2026-10-02T08:00:00Z',
}

// bodiesOf are the bodies a server was sent, by "METHOD /path".
function settingsServer(over: Record<string, (r: Request) => Response | Promise<Response>> = {}) {
  const sent: Record<string, unknown[]> = {}
  // The notes as the server keeps them.
  let notes = 'Knees: no running.'
  const keep = (key: string) => async (r: Request) => {
    const text = await r.text()
    ;(sent[key] ??= []).push(text === '' ? undefined : JSON.parse(text))
  }
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/me': () => Response.json({ ...me(), notes }),
    'GET /api/v1/tokens': () => Response.json({ tokens: [phone, claude] }),
    'GET /api/v1/info': () => Response.json({ name: 'tdeemon', version: 'v0.9.0' }),
    'PATCH /api/v1/me': async (r) => {
      await keep('PATCH /api/v1/me')(r.clone())
      const body: unknown = await r.json()
      return Response.json({
        user: { ...session().user, ...(typeof body === 'object' ? body : {}) },
      })
    },
    'PUT /api/v1/me/notes': async (r) => {
      await keep('PUT /api/v1/me/notes')(r.clone())
      const body: unknown = await r.json()
      if (
        typeof body === 'object' &&
        body !== null &&
        'notes' in body &&
        typeof body.notes === 'string'
      ) {
        notes = body.notes
      }
      return Response.json({ notes })
    },
    'POST /api/v1/tokens': async (r) => {
      await keep('POST /api/v1/tokens')(r)
      return Response.json(
        {
          ...phone,
          id: '0193c3a5-0000-7000-8000-0000000000c3',
          name: 'Laptop',
          token: 'tdm_made_up',
        },
        { status: 201 },
      )
    },
    [`DELETE /api/v1/tokens/${phone.id}`]: async (r) => {
      await keep(`DELETE /api/v1/tokens/${phone.id}`)(r)
      return new Response(null, { status: 204 })
    },
    'DELETE /api/v1/tokens': async (r) => {
      await keep('DELETE /api/v1/tokens')(r)
      return Response.json({ revoked: 2 })
    },
    ...over,
  })
  return { fetch, sent }
}

test("the user's name, language, time zone and day's start are changed, and the app follows", async () => {
  const server = settingsServer()
  renderAt('/settings')

  const you = within(await screen.findByRole('form', { name: 'You' }))
  expect(you.getByLabelText('Name')).toHaveProperty('value', 'Ana')
  fireEvent.change(you.getByLabelText('Name'), { target: { value: ' Ana M. ' } })
  fireEvent.change(you.getByLabelText('Language'), { target: { value: 'hr' } })
  fireEvent.change(you.getByLabelText('Time zone'), { target: { value: 'Europe/Zagreb' } })
  fireEvent.change(you.getByLabelText('The day starts at'), { target: { value: '3' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))

  // Saved, the app speaks Croatian, and the session has the new name.
  expect(await screen.findByRole('heading', { name: 'Postavke' })).toBeDefined()
  expect(screen.getByText('Prijavljeni račun: Ana M. (ana@example.com).')).toBeDefined()
  expect(server.sent['PATCH /api/v1/me']).toEqual([
    { name: 'Ana M.', locale: 'hr', timezone: 'Europe/Zagreb', day_start_hour: 3 },
  ])
})

test('a setting the server refuses says why', async () => {
  settingsServer({
    'PATCH /api/v1/me': () =>
      Response.json(
        {
          status: 422,
          title: 'Unprocessable Entity',
          detail: 'name "‮": contains a control or invisible character',
        },
        { status: 422, headers: { 'Content-Type': 'application/problem+json' } },
      ),
  })
  renderAt('/settings')

  const you = within(await screen.findByRole('form', { name: 'You' }))
  fireEvent.click(you.getByRole('button', { name: 'Save' }))

  expect((await you.findByRole('alert')).textContent).toBe(
    'The server did not take it: name "‮": contains a control or invisible character',
  )
})

test('the notes for a coach are shown and saved', async () => {
  const server = settingsServer()
  renderAt('/settings')

  const notes = await screen.findByLabelText('Notes for a coach')
  expect(notes).toHaveProperty('value', 'Knees: no running.')
  fireEvent.change(notes, { target: { value: 'Knees: no running.\nNo lactose. ' } })
  fireEvent.click(
    within(notes.closest('form') ?? document.body).getByRole('button', { name: 'Save' }),
  )

  await waitFor(() => {
    expect(server.sent['PUT /api/v1/me/notes']).toEqual([
      { notes: 'Knees: no running.\nNo lactose.' },
    ])
  })
  // Saved stays said, and the field as typed.
  expect(await screen.findByText('Saved')).toBeDefined()
  expect(screen.getByLabelText('Notes for a coach')).toHaveProperty(
    'value',
    'Knees: no running.\nNo lactose. ',
  )
})

test('the tokens are listed with their use, and one is made and shown once', async () => {
  const server = settingsServer()
  renderAt('/settings')

  const tokens = within(await screen.findByRole('region', { name: 'API tokens' }))
  expect(await tokens.findByText('iPhone')).toBeDefined()
  expect(tokens.getByText('Send health data · Last used Oct 22 11:00 PM')).toBeDefined()
  expect(tokens.getByText('Read · Log and change · Not used yet')).toBeDefined()
  fireEvent.change(tokens.getByLabelText('Name of the token'), { target: { value: 'Laptop' } })
  fireEvent.click(tokens.getByLabelText('Read'))
  fireEvent.click(tokens.getByRole('button', { name: 'Create a token' }))

  expect(await tokens.findByText('tdm_made_up')).toBeDefined()
  expect(tokens.getByText('The token, shown only now: copy it to the device.')).toBeDefined()
  expect(server.sent['POST /api/v1/tokens']).toEqual([
    { name: 'Laptop', scopes: ['ingest', 'read'] },
  ])
})

test('a token is revoked after it asks, and all of them too', async () => {
  const server = settingsServer()
  renderAt('/settings')

  const tokens = within(await screen.findByRole('region', { name: 'API tokens' }))
  fireEvent.click(await tokens.findByRole('button', { name: 'Revoke: iPhone' }))
  expect(tokens.getByText('Revoke iPhone? It stops working at once.')).toBeDefined()
  fireEvent.click(tokens.getByRole('button', { name: 'Keep' }))
  expect(server.sent[`DELETE /api/v1/tokens/${phone.id}`]).toBeUndefined()
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke: iPhone' }))
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke' }))
  await waitFor(() => {
    expect(server.sent[`DELETE /api/v1/tokens/${phone.id}`]).toHaveLength(1)
  })

  fireEvent.click(tokens.getByRole('button', { name: 'Revoke all tokens' }))
  expect(tokens.getByText(/Revoke every token\?/)).toBeDefined()
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke' }))

  expect(await tokens.findByText('2 tokens revoked.')).toBeDefined()
})

test('no tokens says so, and offers no revoke of all', async () => {
  settingsServer({ 'GET /api/v1/tokens': () => Response.json({ tokens: [] }) })
  renderAt('/settings')

  expect(await screen.findByText('No tokens yet.')).toBeDefined()
  expect(screen.queryByRole('button', { name: 'Revoke all tokens' })).toBeNull()
})

test('tokens that did not come say why', async () => {
  settingsServer({ 'GET /api/v1/tokens': () => problem(500) })
  renderAt('/settings')

  const tokens = within(await screen.findByRole('region', { name: 'API tokens' }))
  expect((await tokens.findByRole('alert')).textContent).toBe('The server failed to answer.')
})
