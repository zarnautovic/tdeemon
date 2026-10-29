import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { setTheme } from '@/lib/theme'
import { nameNow } from '@/lib/time-zones'
import { fakeServer, me, problem, renderAt, session, signedIn } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  setLocale('en')
  setTheme('system')
  localStorage.clear()
})

type Token = components['schemas']['TokenBody']
type User = components['schemas']['UserBody']

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

// settingsServer keeps the user, their notes and their tokens as the
// server does, and the bodies it was sent, by "METHOD /path".
function settingsServer(over: Record<string, (r: Request) => Response | Promise<Response>> = {}) {
  const sent: Record<string, unknown[]> = {}
  let user: User = session().user
  let notes = 'Knees: no running.'
  let tokens = [phone, claude]
  let meFails = false
  const keep = (key: string) => async (r: Request) => {
    const text = await r.text()
    ;(sent[key] ??= []).push(text === '' ? undefined : JSON.parse(text))
  }
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/auth/session': () => Response.json({ ...session(), user }),
    'GET /api/v1/me': () => (meFails ? problem(500) : Response.json({ ...me(), notes })),
    'GET /api/v1/tokens': () => Response.json({ tokens }),
    'GET /api/v1/info': () => Response.json({ name: 'tdeemon', version: 'v0.9.0' }),
    'PATCH /api/v1/me': async (r) => {
      await keep('PATCH /api/v1/me')(r.clone())
      const body: unknown = await r.json()
      user = { ...user, ...(typeof body === 'object' ? body : {}) }
      return Response.json({ user })
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

// summaryOf is what a row of a list says of its value.
function summaryOf(row: HTMLElement): string | null | undefined {
  return document.getElementById(row.getAttribute('aria-describedby') ?? '')?.textContent
}

// account is the form of the account's page.
async function account() {
  return within(await screen.findByRole('form', { name: 'Account, language and day' }))
}

// pick types a search in the time zone's field, and picks a zone it
// lists, as a person does.
async function pick(field: HTMLElement, search: string, zone: string) {
  fireEvent.input(field, { target: { value: search }, inputType: 'insertText' })
  fireEvent.click(await screen.findByRole('option', { name: zone }))
}

// tokensRegion is the devices' page's list of tokens.
async function tokensRegion() {
  return within(await screen.findByRole('region', { name: 'Tokens' }))
}

test('the settings are rows that say their value, each leading to its page and back', async () => {
  settingsServer()
  const router = renderAt('/settings')

  expect(await screen.findByRole('heading', { name: 'Settings', level: 1 })).toBeDefined()
  const appearance = screen.getByRole('link', { name: 'Appearance' })
  expect(summaryOf(appearance)).toBe('System')
  expect(summaryOf(screen.getByRole('link', { name: 'Account, language and day' }))).toBe(
    'English · America/Los_Angeles · the day from 04:00',
  )
  const devices = screen.getByRole('link', { name: 'Devices and AI coaches' })
  await waitFor(() => {
    expect(summaryOf(devices)).toBe('2 tokens')
  })
  expect(summaryOf(screen.getByRole('button', { name: 'Sign out' }))).toBe(
    'Signed in as Ana (ana@example.com).',
  )
  expect((await screen.findByTestId('server-version')).textContent).toBe('v0.9.0')

  fireEvent.click(devices)
  expect(await screen.findByRole('heading', { name: 'Devices and AI coaches' })).toBeDefined()
  expect(router.state.location.pathname).toBe('/settings/devices')
  // Back to the settings on a desktop, to the plan on a phone.
  fireEvent.click(within(screen.getByRole('main')).getByRole('link', { name: 'Settings' }))
  await screen.findByRole('heading', { name: 'Settings', level: 1 })
  fireEvent.click(screen.getByRole('link', { name: 'Appearance' }))
  await screen.findByRole('heading', { name: 'Appearance' })
  fireEvent.click(within(screen.getByRole('main')).getByRole('link', { name: 'Plan' }))
  await screen.findByRole('heading', { name: 'Plan', level: 1 })
  expect(router.state.location.pathname).toBe('/plan')
})

test("the user's name, language, time zone and day's start are changed, and the app follows", async () => {
  const server = settingsServer()
  renderAt('/settings/account')

  const you = await account()
  expect(you.getByLabelText('Name')).toHaveProperty('value', 'Ana')
  expect(you.getByText('Email: ana@example.com')).toBeDefined()
  fireEvent.change(you.getByLabelText('Name'), { target: { value: ' Ana M. ' } })
  fireEvent.change(you.getByLabelText('Language'), { target: { value: 'hr' } })
  await pick(you.getByRole('combobox', { name: 'Time zone' }), 'zagr', 'Europe/Zagreb')
  fireEvent.change(you.getByLabelText('The day starts at'), { target: { value: '3' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))

  // Saved, the app speaks Croatian.
  expect(await screen.findByRole('heading', { name: 'Račun, jezik i dan' })).toBeDefined()
  expect(server.sent['PATCH /api/v1/me']).toEqual([
    { name: 'Ana M.', locale: 'hr', timezone: 'Europe/Zagreb', day_start_hour: 3 },
  ])
})

test("the account's settings are saved, and the page and the list have them back", async () => {
  settingsServer()
  renderAt('/settings/account')

  const you = await account()
  fireEvent.change(you.getByLabelText('Name'), { target: { value: 'Ana M.' } })
  await pick(you.getByRole('combobox', { name: 'Time zone' }), 'london', 'Europe/London')
  fireEvent.change(you.getByLabelText('The day starts at'), { target: { value: '5' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))
  expect(await screen.findByText('Saved')).toBeDefined()

  // The list says them, and the page, opened again, holds them.
  fireEvent.click(within(screen.getByRole('main')).getByRole('link', { name: 'Settings' }))
  const row = await screen.findByRole('link', { name: 'Account, language and day' })
  expect(summaryOf(row)).toBe('English · Europe/London · the day from 05:00')
  expect(summaryOf(screen.getByRole('button', { name: 'Sign out' }))).toBe(
    'Signed in as Ana M. (ana@example.com).',
  )
  fireEvent.click(row)
  const again = await account()
  expect(again.getByLabelText('Name')).toHaveProperty('value', 'Ana M.')
  expect(again.getByRole('combobox', { name: 'Time zone' })).toHaveProperty(
    'value',
    'Europe/London',
  )
  expect(again.getByLabelText('The day starts at')).toHaveProperty('value', '5')
})

test("the theme is the system's until another is chosen, at once and for this browser", async () => {
  const server = settingsServer()
  renderAt('/settings/appearance')

  const theme = within(await screen.findByRole('group', { name: 'Appearance' }))
  expect(theme.getByRole('radio', { name: 'System' })).toHaveProperty('checked', true)
  fireEvent.click(theme.getByRole('radio', { name: 'Dark' }))

  expect(theme.getByRole('radio', { name: 'Dark' })).toHaveProperty('checked', true)
  expect(document.documentElement.classList.contains('dark')).toBe(true)
  expect(localStorage.getItem('tdeemon.theme')).toBe('dark')
  // The browser's, not the user's: nothing goes to the server.
  expect(server.sent['PATCH /api/v1/me']).toBeUndefined()

  fireEvent.click(theme.getByRole('radio', { name: 'Light' }))
  expect(document.documentElement.classList.contains('light')).toBe(true)
  expect(document.documentElement.classList.contains('dark')).toBe(false)
  // The list says the theme chosen.
  fireEvent.click(within(screen.getByRole('main')).getByRole('link', { name: 'Settings' }))
  expect(summaryOf(await screen.findByRole('link', { name: 'Appearance' }))).toBe('Light')
})

test('a name cleared is said to be needed, and not sent', async () => {
  const server = settingsServer()
  renderAt('/settings/account')

  const you = await account()
  fireEvent.change(you.getByLabelText('Name'), { target: { value: '  ' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))

  expect(you.getByRole('alert').textContent).toBe('A name is needed.')
  expect(you.getByLabelText('Name').getAttribute('aria-invalid')).toBe('true')
  expect(server.sent['PATCH /api/v1/me']).toBeUndefined()
  fireEvent.change(you.getByLabelText('Name'), { target: { value: 'Ana' } })
  expect(you.queryByRole('alert')).toBeNull()
})

test('only the settings changed are sent, and the day asked again', async () => {
  const server = settingsServer()
  renderAt('/settings/account')

  const you = await account()
  fireEvent.change(you.getByLabelText('Name'), { target: { value: 'Ana M.' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))

  await screen.findByText('Saved')
  expect(server.sent['PATCH /api/v1/me']).toEqual([{ name: 'Ana M.' }])
  // The day, asked when it is shown again: its bounds may have moved.
  const days = asked(server.fetch, 'GET /api/v1/days/today')
  fireEvent.click(screen.getByRole('link', { name: 'Today' }))
  await waitFor(() => {
    expect(asked(server.fetch, 'GET /api/v1/days/today')).toBeGreaterThan(days)
  })
})

test('the time zones are searched by their names now, and the old ones find them', async () => {
  settingsServer()
  renderAt('/settings/account')

  const zone = (await account()).getByRole('combobox', { name: 'Time zone' })
  expect(zone).toHaveProperty('value', 'America/Los_Angeles')
  // All of them, UTC and the browser's own among them.
  fireEvent.click(screen.getByRole('button', { name: 'All time zones' }))
  const all = (await screen.findAllByRole('option')).map((o) => o.textContent)
  expect(all).toContain('UTC')
  // Under its name now: a browser in Kolkata says Asia/Calcutta.
  expect(all).toContain(nameNow(Intl.DateTimeFormat().resolvedOptions().timeZone))
  expect(all).toContain('America/Los_Angeles')
  expect(all).toContain('Asia/Kolkata')
  expect(all).not.toContain('Asia/Calcutta')
  expect(all.length).toBeGreaterThan(300)

  // A space for an underscore, and an old name finds the zone's name now.
  fireEvent.input(zone, { target: { value: 'new york' }, inputType: 'insertText' })
  await waitFor(() => {
    expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual(['America/New_York'])
  })
  fireEvent.input(zone, { target: { value: 'Kiev' }, inputType: 'insertText' })
  await waitFor(() => {
    expect(screen.getAllByRole('option').map((o) => o.textContent)).toEqual(['Europe/Kyiv'])
  })
  fireEvent.input(zone, { target: { value: 'Atlantis' }, inputType: 'insertText' })
  expect(await screen.findByText('No time zone of that name.')).toBeDefined()
  expect(screen.queryAllByRole('option')).toHaveLength(0)
})

test("the time zone is the browser's at a tap, and is saved", async () => {
  // The browser is far from the user's zone.
  vi.spyOn(Intl.DateTimeFormat.prototype, 'resolvedOptions').mockReturnValue({
    ...new Intl.DateTimeFormat().resolvedOptions(),
    timeZone: 'Asia/Tokyo',
  })
  const zoneHere = 'Asia/Tokyo'
  const server = settingsServer()
  renderAt('/settings/account')

  const you = await account()
  const detect = you.getByRole('button', { name: 'Detect automatically' })
  fireEvent.click(detect)
  expect(you.getByRole('combobox', { name: 'Time zone' })).toHaveProperty('value', zoneHere)
  // The zone is the browser's now: nothing more to detect.
  expect(detect).toHaveProperty('disabled', true)
  fireEvent.click(you.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(server.sent['PATCH /api/v1/me']).toEqual([{ timezone: zoneHere }])
  })
})

test('a browser without the list of zones has a field to type one in', async () => {
  vi.stubGlobal('Intl', Object.create(Intl, { supportedValuesOf: { value: undefined } }))
  const server = settingsServer()
  renderAt('/settings/account')

  const typed = (await account()).getByRole('textbox', { name: 'Time zone' })
  expect(typed).toHaveProperty('value', 'America/Los_Angeles')
  fireEvent.change(typed, { target: { value: 'Europe/Zagreb' } })
  fireEvent.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(server.sent['PATCH /api/v1/me']).toEqual([{ timezone: 'Europe/Zagreb' }])
  })
})

test("a user's zone under its old name is still theirs, and kept unless changed", async () => {
  const server = settingsServer({
    'GET /api/v1/auth/session': () =>
      Response.json({ ...session(), user: { ...session().user, timezone: 'Europe/Kiev' } }),
  })
  renderAt('/settings/account')

  const you = await account()
  expect(you.getByRole('combobox', { name: 'Time zone' })).toHaveProperty('value', 'Europe/Kiev')
  fireEvent.change(you.getByLabelText('Name'), { target: { value: 'Ana M.' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(server.sent['PATCH /api/v1/me']).toEqual([{ name: 'Ana M.' }])
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
  renderAt('/settings/account')

  const you = await account()
  fireEvent.change(you.getByLabelText('Name'), { target: { value: 'Ana M.' } })
  fireEvent.click(you.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(you.getByRole('combobox', { name: 'Time zone' })).toHaveProperty(
      'value',
      'Europe/London',
    )
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
  renderAt('/settings/account')

  const you = await account()
  fireEvent.click(you.getByRole('button', { name: 'Save' }))

  expect((await you.findByRole('alert')).textContent).toBe(
    'The server did not take it: name "‮": contains a control or invisible character',
  )
})

test('the notes for a coach are shown and saved, and the page and the list have them back', async () => {
  const server = settingsServer()
  renderAt('/plan/notes')

  const notes = await screen.findByRole('textbox', { name: 'Notes for your AI coach' })
  expect(notes).toHaveProperty('value', 'Knees: no running.')
  fireEvent.change(notes, { target: { value: 'No lactose.\nKnees: no running. ' } })
  fireEvent.click(
    within(screen.getByRole('form', { name: 'Notes for your AI coach' })).getByRole('button', {
      name: 'Save',
    }),
  )

  await waitFor(() => {
    expect(server.sent['PUT /api/v1/me/notes']).toEqual([
      { notes: 'No lactose.\nKnees: no running.' },
    ])
  })
  // A toast says it is saved, and the field has the notes as the server
  // keeps them.
  expect(await screen.findByText('The notes for your AI coach are saved')).toBeDefined()
  expect(screen.getByRole('textbox', { name: 'Notes for your AI coach' })).toHaveProperty(
    'value',
    'No lactose.\nKnees: no running.',
  )
  // The plan's list says their first line, and the page, opened again,
  // has them.
  fireEvent.click(within(screen.getByRole('main')).getByRole('link', { name: 'Plan' }))
  const row = await screen.findByRole('link', { name: 'Notes for your AI coach' })
  await waitFor(() => {
    expect(summaryOf(row)).toBe('No lactose.')
  })
  fireEvent.click(row)
  expect(await screen.findByRole('textbox', { name: 'Notes for your AI coach' })).toHaveProperty(
    'value',
    'No lactose.\nKnees: no running.',
  )
})

test('notes taken back by their toast are taken back in the field, and Save sends them', async () => {
  const server = settingsServer({
    'POST /api/v1/undo': () => {
      server.setNotes('Knees: no running.')
      return Response.json({
        undone: {
          action: 'notes.set',
          entity: 'notes',
          at: '2026-10-23T16:00:00Z',
          client: 'web',
          days: [],
        },
      })
    },
  })
  renderAt('/plan/notes')
  const form = within(await screen.findByRole('form', { name: 'Notes for your AI coach' }))

  fireEvent.change(await form.findByRole('textbox'), {
    target: { value: 'Knees: no running. No broccoli.' },
  })
  fireEvent.click(form.getByRole('button', { name: 'Save' }))
  expect(await screen.findByText('The notes for your AI coach are saved')).toBeDefined()
  fireEvent.click(screen.getByRole('button', { name: 'Undo' }))

  expect(await screen.findByText('Taken back: the notes for your AI coach.')).toBeDefined()
  await waitFor(() => {
    expect(form.getByRole('textbox')).toHaveProperty('value', 'Knees: no running.')
  })
  fireEvent.click(form.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(server.sent['PUT /api/v1/me/notes']).toEqual([
      { notes: 'Knees: no running. No broccoli.' },
      { notes: 'Knees: no running.' },
    ])
  })
})

test('notes typed after a save stay, though the save comes back', async () => {
  // The save waits here until the test lets it through.
  const answers: (() => void)[] = []
  const server = settingsServer({
    'PUT /api/v1/me/notes': async (r) => {
      const body: unknown = await r.json()
      const sent =
        typeof body === 'object' &&
        body !== null &&
        'notes' in body &&
        typeof body.notes === 'string'
          ? body.notes
          : ''
      await new Promise<void>((resolve) => {
        answers.push(resolve)
      })
      server.setNotes(sent)
      return Response.json({ notes: sent })
    },
  })
  renderAt('/plan/notes')
  const form = within(await screen.findByRole('form', { name: 'Notes for your AI coach' }))

  fireEvent.change(await form.findByRole('textbox'), { target: { value: 'No lactose.' } })
  fireEvent.click(form.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(answers).toHaveLength(1)
  })
  // Typed while the save is on its way.
  fireEvent.change(form.getByRole('textbox'), { target: { value: 'No lactose. No nuts.' } })
  answers[0]?.()

  expect(await screen.findByText('The notes for your AI coach are saved')).toBeDefined()
  expect(form.getByRole('textbox')).toHaveProperty('value', 'No lactose. No nuts.')
})

test("the notes are the server's, not those a cache kept from before", async () => {
  const server = settingsServer()
  renderAt('/plan/notes')

  expect(await screen.findByRole('textbox', { name: 'Notes for your AI coach' })).toHaveProperty(
    'value',
    'Knees: no running.',
  )
  fireEvent.click(screen.getByRole('link', { name: 'Today' }))
  await screen.findByRole('heading', { name: 'Today' })
  // An undo through a model took them back meanwhile.
  server.setNotes('')
  fireEvent.click(screen.getByRole('link', { name: 'Plan' }))
  const row = await screen.findByRole('link', { name: 'Notes for your AI coach' })
  await waitFor(() => {
    expect(summaryOf(row)).toBe('None yet')
  })
  fireEvent.click(row)
  await waitFor(() => {
    expect(screen.getByRole('textbox', { name: 'Notes for your AI coach' })).toHaveProperty(
      'value',
      '',
    )
  })
})

test('notes typed stay when the user cannot be asked again', async () => {
  const server = settingsServer()
  renderAt('/plan/notes')

  const notes = await screen.findByRole('textbox', { name: 'Notes for your AI coach' })
  fireEvent.change(notes, { target: { value: 'Knees: no running. Shoulder too.' } })
  // Saved, the user is asked again, which fails.
  server.failMe()
  const before = asked(server.fetch, 'GET /api/v1/me')
  fireEvent.click(
    within(screen.getByRole('form', { name: 'Notes for your AI coach' })).getByRole('button', {
      name: 'Save',
    }),
  )
  await waitFor(() => {
    expect(asked(server.fetch, 'GET /api/v1/me')).toBeGreaterThan(before)
  })
  expect(screen.getByRole('textbox', { name: 'Notes for your AI coach' })).toHaveProperty(
    'value',
    'Knees: no running. Shoulder too.',
  )
  expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
})

test("notes the server failed to give again are not the cache's", async () => {
  const server = settingsServer()
  renderAt('/plan/notes')

  await screen.findByRole('textbox', { name: 'Notes for your AI coach' })
  fireEvent.click(screen.getByRole('link', { name: 'Today' }))
  await screen.findByRole('heading', { name: 'Today' })
  server.failMe()
  fireEvent.click(screen.getByRole('link', { name: 'Plan' }))
  fireEvent.click(await screen.findByRole('link', { name: 'Notes for your AI coach' }))
  await screen.findByRole('heading', { name: 'Notes for your AI coach' })
  expect(await screen.findByText('The server failed to answer.')).toBeDefined()
  expect(screen.queryByRole('textbox', { name: 'Notes for your AI coach' })).toBeNull()
  expect(screen.getByRole('button', { name: 'Try again' })).toBeDefined()
})

test('the tokens are cards with what each may do and when it was used', async () => {
  settingsServer()
  renderAt('/settings/devices')

  const tokens = await tokensRegion()
  await tokens.findByRole('heading', { name: 'iPhone' })
  const card = (name: string) => {
    const li = tokens.getByRole('heading', { name }).closest('li')
    if (!li) {
      throw new Error(`no card ${name}`)
    }
    return within(li)
  }
  const scopes = (name: string) =>
    within(card(name).getByRole('list', { name: 'What it may do' }))
      .getAllByRole('listitem')
      .map((li) => li.textContent)
  expect(card('iPhone').getByText('Last used Oct 22 11:00 PM')).toBeDefined()
  expect(scopes('iPhone')).toEqual(['Send health data'])
  expect(card('Claude').getByText('Not used yet')).toBeDefined()
  expect(scopes('Claude')).toEqual(['Read', 'Read, log and change'])
})

test('a new token is made in a sheet, shown once and copied', async () => {
  const server = settingsServer()
  const writeText = vi.fn<(s: string) => Promise<void>>(() => Promise.resolve())
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  renderAt('/settings/devices')

  const tokens = await tokensRegion()
  fireEvent.click(await tokens.findByRole('button', { name: 'New token' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'New token' }))
  const name = sheet.getByLabelText('Name')
  // The field has the focus, and a placeholder of no token of the user's.
  await waitFor(() => {
    expect(document.activeElement).toBe(name)
  })
  expect(name.getAttribute('placeholder')).toBe('My phone')
  // The region that says the token came is there before it, so it is
  // said; the token itself is not read out.
  const live = sheet.getByRole('status')
  expect(live.textContent).toBe('')
  fireEvent.change(name, { target: { value: 'Laptop' } })
  fireEvent.click(sheet.getByLabelText('Read'))
  fireEvent.click(sheet.getByRole('button', { name: 'Create the token' }))

  expect(await sheet.findByText('tdm_made_up')).toBeDefined()
  expect(live.textContent).toBe('The token, shown only now: copy it to the device.')
  expect(server.sent['POST /api/v1/tokens']).toEqual([
    { name: 'Laptop', scopes: ['ingest', 'read'] },
  ])
  // Copy takes the focus, and copies it.
  const copy = sheet.getByRole('button', { name: 'Copy' })
  expect(document.activeElement).toBe(copy)
  fireEvent.click(copy)
  expect(await sheet.findByRole('button', { name: 'Copied' })).toBeDefined()
  expect(writeText).toHaveBeenCalledWith('tdm_made_up')

  // Done, it is shown no more, anywhere, and the list has it; a new
  // sheet starts empty.
  fireEvent.click(sheet.getByRole('button', { name: 'Done' }))
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).toBeNull()
  })
  expect(screen.queryByText('tdm_made_up')).toBeNull()
  expect(await tokens.findByRole('heading', { name: 'Laptop' })).toBeDefined()
  fireEvent.click(tokens.getByRole('button', { name: 'New token' }))
  const again = within(await screen.findByRole('dialog', { name: 'New token' }))
  expect(again.getByLabelText('Name')).toHaveProperty('value', '')
  expect(again.queryByText('tdm_made_up')).toBeNull()
  Reflect.deleteProperty(navigator, 'clipboard')
})

// QA #253: a token made from the plan's Connect an AI coach only sent
// health data, which MCP refuses.
test("an AI coach's token, asked for by the plan, may read, log and change", async () => {
  const server = settingsServer()
  const router = renderAt('/settings/devices?connect=coach')

  // The sheet opens at once, and the address asks no more.
  const sheet = within(await screen.findByRole('dialog', { name: 'New token' }))
  const name = sheet.getByLabelText('Name')
  expect(name.getAttribute('placeholder')).toBe('My AI coach')
  expect(sheet.getByLabelText('Read, log and change')).toHaveProperty('checked', true)
  expect(sheet.getByLabelText('Send health data')).toHaveProperty('checked', false)
  await waitFor(() => {
    expect(router.state.location.search).toEqual({})
  })
  expect(screen.getByRole('dialog', { name: 'New token' })).toBeDefined()

  fireEvent.change(name, { target: { value: 'Claude' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Create the token' }))
  expect(await sheet.findByText('tdm_made_up')).toBeDefined()
  expect(server.sent['POST /api/v1/tokens']).toEqual([{ name: 'Claude', scopes: ['tools'] }])

  // A token made from the page's button is a device's again.
  fireEvent.click(sheet.getByRole('button', { name: 'Done' }))
  await waitFor(() => {
    expect(screen.queryByRole('dialog')).toBeNull()
  })
  fireEvent.click(await (await tokensRegion()).findByRole('button', { name: 'New token' }))
  const again = within(await screen.findByRole('dialog', { name: 'New token' }))
  expect(again.getByLabelText('Send health data')).toHaveProperty('checked', true)
  expect(again.getByLabelText('Read, log and change')).toHaveProperty('checked', false)
})

test('a token the browser cannot copy is selected to copy by hand', async () => {
  settingsServer()
  // Over plain http there is no clipboard.
  Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
  renderAt('/settings/devices')

  fireEvent.click(await (await tokensRegion()).findByRole('button', { name: 'New token' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'New token' }))
  fireEvent.change(sheet.getByLabelText('Name'), { target: { value: 'Laptop' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Create the token' }))
  fireEvent.click(await sheet.findByRole('button', { name: 'Copy' }))
  expect((await sheet.findByRole('alert')).textContent).toBe(
    'Copying failed: the token is selected, copy it by hand.',
  )
  expect(window.getSelection()?.toString()).toBe('tdm_made_up')
  Reflect.deleteProperty(navigator, 'clipboard')
})

test('a token is not made without a name or anything it may do', async () => {
  const server = settingsServer()
  renderAt('/settings/devices')

  fireEvent.click(await (await tokensRegion()).findByRole('button', { name: 'New token' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'New token' }))
  const create = sheet.getByRole('button', { name: 'Create the token' })
  expect(create).toHaveProperty('disabled', true)
  fireEvent.change(sheet.getByLabelText('Name'), { target: { value: 'Laptop' } })
  expect(create).toHaveProperty('disabled', false)
  fireEvent.click(sheet.getByLabelText('Send health data'))
  expect(create).toHaveProperty('disabled', true)
  expect(server.sent['POST /api/v1/tokens']).toBeUndefined()
})

test('a token is revoked after it asks, and all of them too', async () => {
  const server = settingsServer()
  renderAt('/settings/devices')

  const tokens = await tokensRegion()
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
  expect(await tokens.findByText('No device or client yet.')).toBeDefined()
})

test('a question to revoke takes the focus to Keep, and gives it back', async () => {
  settingsServer()
  renderAt('/settings/devices')

  const tokens = await tokensRegion()
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
    expect(document.activeElement).toBe(tokens.getByRole('heading', { name: 'Tokens' }))
  })
})

test('a token revoked elsewhere already is gone, not a failure', async () => {
  const server = settingsServer()
  renderAt('/settings/devices')

  const tokens = await tokensRegion()
  fireEvent.click(await tokens.findByRole('button', { name: 'Revoke: iPhone' }))
  server.setTokens([claude])
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke' }))

  await waitFor(() => {
    expect(tokens.queryByText('iPhone')).toBeNull()
  })
  expect(tokens.queryByRole('alert')).toBeNull()
  expect(tokens.queryByText('Revoke iPhone? It stops working at once.')).toBeNull()
})

test('a new token after revoking all says no more what that came to', async () => {
  settingsServer()
  renderAt('/settings/devices')

  const tokens = await tokensRegion()
  fireEvent.click(await tokens.findByRole('button', { name: 'Revoke all tokens' }))
  fireEvent.click(tokens.getByRole('button', { name: 'Revoke' }))
  expect(await tokens.findByText('2 tokens revoked.')).toBeDefined()

  fireEvent.click(tokens.getByRole('button', { name: 'New token' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'New token' }))
  fireEvent.change(sheet.getByLabelText('Name'), { target: { value: 'Laptop' } })
  fireEvent.click(sheet.getByRole('button', { name: 'Create the token' }))
  expect(await sheet.findByText('tdm_made_up')).toBeDefined()
  expect(tokens.queryByText('2 tokens revoked.')).toBeNull()
})

test('no tokens says so, with the way to make one, and offers no revoke of all', async () => {
  settingsServer({ 'GET /api/v1/tokens': () => Response.json({ tokens: [] }) })
  renderAt('/settings/devices')

  const tokens = await tokensRegion()
  expect(await tokens.findByText('No device or client yet.')).toBeDefined()
  expect(tokens.getAllByRole('button', { name: 'New token' })).toHaveLength(1)
  expect(tokens.queryByRole('button', { name: 'Revoke all tokens' })).toBeNull()
})

test('tokens that did not come say why, and come when asked again', async () => {
  let fails = true
  settingsServer({
    'GET /api/v1/tokens': () => (fails ? problem(500) : Response.json({ tokens: [phone] })),
  })
  renderAt('/settings/devices')

  const tokens = await tokensRegion()
  expect((await tokens.findByRole('alert')).textContent).toBe('The server failed to answer.')
  fails = false
  fireEvent.click(tokens.getByRole('button', { name: 'Try again' }))
  expect(await tokens.findByRole('heading', { name: 'iPhone' })).toBeDefined()
})
