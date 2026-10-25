import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { nameNow } from '@/lib/time-zones'
import { fakeServer, me, problem, renderAt, session, signedIn } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

type Token = components['schemas']['TokenBody']

const phone: Token = {
  id: '0193c3a5-0000-7000-8000-0000000000c1',
  name: 'iPhone',
  scopes: ['ingest' as const],
  created_at: '2026-10-01T08:00:00Z',
  last_used_at: '2026-10-23T06:00:00Z',
}
const laptop = {
  id: '0193c3a5-0000-7000-8000-0000000000c3',
  name: 'Laptop',
  scopes: ['ingest' as const, 'read' as const],
  created_at: '2026-10-23T08:00:00Z',
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
  // The notes and the tokens as the server keeps them.
  let notes = 'Knees: no running.'
  let tokens = [phone, claude]
  let meFails = false
  const keep = (key: string) => async (r: Request) => {
    const text = await r.text()
    ;(sent[key] ??= []).push(text === '' ? undefined : JSON.parse(text))
  }
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/me': () => (meFails ? problem(500) : Response.json({ ...me(), notes })),
    'GET /api/v1/tokens': () => Response.json({ tokens }),
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
      tokens = [...tokens, laptop]
      return Response.json({ ...laptop, token: 'tdm_made_up' }, { status: 201 })
    },
    ...Object.fromEntries(
      [phone, claude, laptop].map((tok) => [
        `DELETE /api/v1/tokens/${tok.id}`,
        async (r: Request) => {
          await keep(`DELETE /api/v1/tokens/${tok.id}`)(r)
          if (!tokens.some((x) => x.id === tok.id)) {
            return problem(404)
          }
          tokens = tokens.filter((x) => x.id !== tok.id)
          return new Response(null, { status: 204 })
        },
      ]),
    ),
    'DELETE /api/v1/tokens': async (r) => {
      await keep('DELETE /api/v1/tokens')(r)
      const revoked = tokens.length
      tokens = []
      return Response.json({ revoked })
    },
    ...over,
  })
  return {
    fetch,
    sent,
    setNotes: (n: string) => (notes = n),
    failMe: () => (meFails = true),
    setTokens: (ts: Token[]) => (tokens = ts),
  }
}

// asked counts the requests a server was sent, by "METHOD /path".
function asked(fetch: ReturnType<typeof fakeServer>, key: string): number {
  return fetch.mock.calls.filter(([input, init]) => {
    const request = input instanceof Request ? input : new Request(input, init)
    return `${request.method} ${new URL(request.url).pathname}` === key
  }).length
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

test('only the settings changed are sent, and the days and the user asked again', async () => {
  const server = settingsServer()
  renderAt('/settings')

  const you = within(await screen.findByRole('form', { name: 'You' }))
  const before = asked(server.fetch, 'GET /api/v1/me')
  fireEvent.change(you.getByLabelText('Name'), { target: { value: 'Ana M.' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))

  await screen.findByText('Signed in as Ana M. (ana@example.com).')
  expect(server.sent['PATCH /api/v1/me']).toEqual([{ name: 'Ana M.' }])
  await waitFor(() => {
    expect(asked(server.fetch, 'GET /api/v1/me')).toBeGreaterThan(before)
  })
  // The day, asked when it is shown again: its bounds may have moved.
  const days = asked(server.fetch, 'GET /api/v1/days/today')
  fireEvent.click(screen.getByRole('link', { name: 'Day' }))
  await waitFor(() => {
    expect(asked(server.fetch, 'GET /api/v1/days/today')).toBeGreaterThan(days)
  })
})

test("the time zones offer UTC and the browser's own, or a field without a list", async () => {
  settingsServer()
  renderAt('/settings')

  const zone = within(await screen.findByRole('form', { name: 'You' })).getByLabelText('Time zone')
  const options = within(zone)
    .getAllByRole('option')
    .map((o) => o.textContent)
  expect(options).toContain('UTC')
  // Under its name now: a browser in Kolkata says Asia/Calcutta.
  expect(options).toContain(nameNow(Intl.DateTimeFormat().resolvedOptions().timeZone))
  expect(options).toContain('America/Los_Angeles')
  // Under the names IANA gives them now, though the browser keeps old ones.
  expect(options).toContain('Europe/Kyiv')
  expect(options).not.toContain('Europe/Kiev')
  expect(options).toContain('Asia/Kolkata')
  expect(options).not.toContain('Asia/Calcutta')
  cleanup()

  vi.stubGlobal('Intl', Object.create(Intl, { supportedValuesOf: { value: undefined } }))
  const server = settingsServer()
  renderAt('/settings')
  const typed = within(await screen.findByRole('form', { name: 'You' })).getByRole('textbox', {
    name: 'Time zone',
  })
  expect(typed).toHaveProperty('value', 'America/Los_Angeles')
  fireEvent.change(typed, { target: { value: 'Europe/Zagreb' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(server.sent['PATCH /api/v1/me']).toEqual([{ timezone: 'Europe/Zagreb' }])
  })
  // The rest of the page stands: the tokens' times are formatted.
  expect(await screen.findByText('iPhone')).toBeDefined()
})

test("a user's zone under its old name is still theirs, and kept unless changed", async () => {
  const server = settingsServer({
    'GET /api/v1/auth/session': () =>
      Response.json({ ...session(), user: { ...session().user, timezone: 'Europe/Kiev' } }),
  })
  renderAt('/settings')

  const you = within(await screen.findByRole('form', { name: 'You' }))
  expect(you.getByLabelText('Time zone')).toHaveProperty('value', 'Europe/Kiev')
  fireEvent.change(you.getByLabelText('Name'), { target: { value: 'Ana M.' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(server.sent['PATCH /api/v1/me']).toEqual([{ name: 'Ana M.' }])
  })
})

test("the notes are the server's, not those a cache kept from before", async () => {
  const server = settingsServer()
  renderAt('/settings')

  expect(await screen.findByLabelText('Notes for a coach')).toHaveProperty(
    'value',
    'Knees: no running.',
  )
  fireEvent.click(screen.getByRole('link', { name: 'Day' }))
  await screen.findByRole('heading', { name: 'Today' })
  // An undo through a model took them back meanwhile.
  server.setNotes('')
  fireEvent.click(screen.getByRole('link', { name: 'Settings' }))
  await screen.findByRole('heading', { name: 'Settings' })
  await waitFor(() => {
    expect(screen.getByLabelText('Notes for a coach')).toHaveProperty('value', '')
  })
})

test('a setting changed elsewhere shows, and is not sent back', async () => {
  // The user as the server has them: another tab moved the zone, and
  // later renames them, which the answers bring.
  let user = { ...session().user, timezone: 'Europe/London' }
  const server = settingsServer({
    'PATCH /api/v1/me': async (r) => {
      const body: unknown = await r.json()
      ;(server.sent['PATCH /api/v1/me'] ??= []).push(body)
      user = { ...user, ...(typeof body === 'object' ? body : {}) }
      const answer = { user }
      user = { ...user, name: 'Ana K.' }
      return Response.json(answer)
    },
  })
  renderAt('/settings')

  const you = within(await screen.findByRole('form', { name: 'You' }))
  fireEvent.change(you.getByLabelText('Name'), { target: { value: 'Ana M.' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(you.getByLabelText('Time zone')).toHaveProperty('value', 'Europe/London')
  })
  fireEvent.change(you.getByLabelText('The day starts at'), { target: { value: '3' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(server.sent['PATCH /api/v1/me']).toEqual([{ name: 'Ana M.' }, { day_start_hour: 3 }])
  })
  // The name the other tab gave shows, and the next change leaves it.
  await waitFor(() => {
    expect(you.getByLabelText('Name')).toHaveProperty('value', 'Ana K.')
  })
  fireEvent.change(you.getByLabelText('The day starts at'), { target: { value: '5' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(server.sent['PATCH /api/v1/me']).toEqual([
      { name: 'Ana M.' },
      { day_start_hour: 3 },
      { day_start_hour: 5 },
    ])
  })
})

test('notes typed stay when the user cannot be asked again', async () => {
  const server = settingsServer()
  renderAt('/settings')

  const notes = await screen.findByLabelText('Notes for a coach')
  fireEvent.change(notes, { target: { value: 'Knees: no running. Shoulder too.' } })
  // A change of the name asks for the user again, which fails.
  server.failMe()
  const before = asked(server.fetch, 'GET /api/v1/me')
  const you = within(screen.getByRole('form', { name: 'You' }))
  fireEvent.change(you.getByLabelText('Name'), { target: { value: 'Ana M.' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))
  await screen.findByText('Signed in as Ana M. (ana@example.com).')
  await waitFor(() => {
    expect(asked(server.fetch, 'GET /api/v1/me')).toBeGreaterThan(before)
  })
  expect(screen.getByLabelText('Notes for a coach')).toHaveProperty(
    'value',
    'Knees: no running. Shoulder too.',
  )
})

test("notes the server failed to give again are not the cache's", async () => {
  const server = settingsServer()
  renderAt('/settings')

  await screen.findByLabelText('Notes for a coach')
  fireEvent.click(screen.getByRole('link', { name: 'Day' }))
  await screen.findByRole('heading', { name: 'Today' })
  server.failMe()
  fireEvent.click(screen.getByRole('link', { name: 'Settings' }))
  await screen.findByRole('heading', { name: 'Settings' })
  expect(await screen.findByText('The server failed to answer.')).toBeDefined()
  expect(screen.queryByLabelText('Notes for a coach')).toBeNull()
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
  expect(tokens.getByText('Read · Read, log and change · Not used yet')).toBeDefined()
  // The region that says the token came is there before it, so it is
  // said; the token itself is not read out.
  const live = document.querySelector('form output[id$="-made"]')
  expect(live?.textContent).toBe('')
  fireEvent.change(tokens.getByLabelText('Name of the token'), { target: { value: 'Laptop' } })
  fireEvent.click(tokens.getByLabelText('Read'))
  fireEvent.click(tokens.getByRole('button', { name: 'Create a token' }))

  expect(await tokens.findByText('tdm_made_up')).toBeDefined()
  expect(live?.isConnected).toBe(true)
  expect(live?.textContent).toBe('The token, shown only now: copy it to the device.')
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
  // Revoked, the list is asked again, without it.
  await waitFor(() => {
    expect(tokens.queryByText('iPhone')).toBeNull()
  })
  expect(server.sent[`DELETE /api/v1/tokens/${phone.id}`]).toHaveLength(1)

  // What revoking all comes to is said in a region there before it.
  const said = document.querySelector('section[aria-labelledby="settings-tokens"] > output')
  expect(said?.textContent).toBe('')
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke all tokens' }))
  expect(tokens.getByText(/Revoke every token\?/)).toBeDefined()
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke' }))

  expect(await tokens.findByText('1 token revoked.')).toBeDefined()
  expect(said?.isConnected).toBe(true)
  expect(said?.textContent).toBe('1 token revoked.')
  expect(await tokens.findByText('No tokens yet.')).toBeDefined()
})

test('a question to revoke takes the focus to Keep, and gives it back', async () => {
  settingsServer()
  renderAt('/settings')

  const tokens = within(await screen.findByRole('region', { name: 'API tokens' }))
  fireEvent.click(await tokens.findByRole('button', { name: 'Revoke: iPhone' }))
  const keep = tokens.getByRole('button', { name: 'Keep' })
  expect(document.activeElement).toBe(keep)
  expect(keep.getAttribute('aria-describedby')).toBeTruthy()
  expect(document.getElementById(keep.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Revoke iPhone? It stops working at once.',
  )
  fireEvent.click(keep)
  expect(document.activeElement).toBe(tokens.getByRole('button', { name: 'Revoke: iPhone' }))

  fireEvent.click(tokens.getByRole('button', { name: 'Revoke all tokens' }))
  expect(document.activeElement).toBe(tokens.getByRole('button', { name: 'Keep' }))
  fireEvent.click(tokens.getByRole('button', { name: 'Keep' }))
  expect(document.activeElement).toBe(tokens.getByRole('button', { name: 'Revoke all tokens' }))

  // A token revoked is gone, and the focus goes to the heading.
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke: iPhone' }))
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke' }))
  await waitFor(() => {
    expect(document.activeElement).toBe(tokens.getByRole('heading', { name: 'API tokens' }))
  })
})

test('a token revoked elsewhere already is gone, not a failure', async () => {
  const server = settingsServer()
  renderAt('/settings')

  const tokens = within(await screen.findByRole('region', { name: 'API tokens' }))
  fireEvent.click(await tokens.findByRole('button', { name: 'Revoke: iPhone' }))
  server.setTokens([claude])
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke' }))

  await waitFor(() => {
    expect(tokens.queryByText('iPhone')).toBeNull()
  })
  expect(tokens.queryByRole('alert')).toBeNull()
  expect(tokens.queryByText('Revoke iPhone? It stops working at once.')).toBeNull()
})

test('a token made is in the list, and shown until it no longer works', async () => {
  settingsServer()
  renderAt('/settings')

  const tokens = within(await screen.findByRole('region', { name: 'API tokens' }))
  fireEvent.change(await tokens.findByLabelText('Name of the token'), {
    target: { value: 'Laptop' },
  })
  fireEvent.click(tokens.getByRole('button', { name: 'Create a token' }))
  expect(await tokens.findByText('tdm_made_up')).toBeDefined()
  expect(await tokens.findByRole('button', { name: 'Revoke: Laptop' })).toBeDefined()

  fireEvent.click(tokens.getByRole('button', { name: 'Revoke all tokens' }))
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke' }))
  expect(await tokens.findByText('3 tokens revoked.')).toBeDefined()
  expect(tokens.queryByText('tdm_made_up')).toBeNull()

  // A new one, and what revoking came to is no longer said.
  fireEvent.change(tokens.getByLabelText('Name of the token'), { target: { value: 'Laptop' } })
  fireEvent.click(tokens.getByRole('button', { name: 'Create a token' }))
  expect(await tokens.findByText('tdm_made_up')).toBeDefined()
  expect(tokens.queryByText('3 tokens revoked.')).toBeNull()

  // Revoking it hides it too.
  fireEvent.click(await tokens.findByRole('button', { name: 'Revoke: Laptop' }))
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke' }))
  await waitFor(() => {
    expect(tokens.queryByText('tdm_made_up')).toBeNull()
  })
})

test('a token is copied, or selected where the browser cannot copy it', async () => {
  settingsServer()
  const writeText = vi.fn<(s: string) => Promise<void>>(() => Promise.resolve())
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  renderAt('/settings')

  const tokens = within(await screen.findByRole('region', { name: 'API tokens' }))
  fireEvent.change(await tokens.findByLabelText('Name of the token'), {
    target: { value: 'Laptop' },
  })
  fireEvent.click(tokens.getByRole('button', { name: 'Create a token' }))
  fireEvent.click(await tokens.findByRole('button', { name: 'Copy' }))
  expect(await tokens.findByRole('button', { name: 'Copied' })).toBeDefined()
  expect(writeText).toHaveBeenCalledWith('tdm_made_up')

  // Over plain http there is no clipboard.
  Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  fireEvent.click(tokens.getByRole('button', { name: 'Copied' }))
  expect((await tokens.findByRole('alert')).textContent).toBe(
    'Copying failed: the token is selected, copy it by hand.',
  )
  expect(window.getSelection()?.toString()).toBe('tdm_made_up')
  Reflect.deleteProperty(navigator, 'clipboard')
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
