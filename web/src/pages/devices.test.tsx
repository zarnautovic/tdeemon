import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, signedIn } from '@/test/app'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
})

type Key = components['schemas']['TokenBody']

const made: Key = {
  id: '0193c3a5-0000-7000-8000-0000000000c4',
  name: 'Claude',
  scopes: ['tools'],
  created_at: '2026-10-23T08:00:00Z',
}

// connectServer keeps the user's access keys as the server does, and
// what it was sent to make one. info is what the server says of itself.
function connectServer(info: () => Response = () => Response.json(instance), locale?: 'hr') {
  const sent: unknown[] = []
  let keys: Key[] = []
  fakeServer({
    ...signedIn(locale),
    'GET /api/v1/info': info,
    'GET /api/v1/tokens': () => Response.json({ tokens: keys }),
    'POST /api/v1/tokens': async (r) => {
      const body: unknown = await r.json()
      sent.push(body)
      const scopes =
        typeof body === 'object' && body !== null && 'scopes' in body && Array.isArray(body.scopes)
          ? body.scopes.filter((s): s is Key['scopes'][number] => typeof s === 'string')
          : []
      keys = [{ ...made, scopes }]
      return Response.json({ ...made, scopes, token: 'tdm_made_up' }, { status: 201 })
    },
  })
  return {
    sent,
    // called is the key used, as a client's first call marks it.
    called: (at: string) => {
      for (const k of keys) {
        k.last_used_at = at
      }
    },
  }
}

const instance = { name: 'tdeemon', version: 'v0.9.0', base_url: 'https://tdeemon.example.com' }

// valueOf is the text a copy field of the open sheet shows, by its label.
function valueOf(label: string): string | null {
  return (
    within(screen.getByRole('dialog')).getByRole('group', { name: label }).querySelector('code')
      ?.textContent ?? null
  )
}

test("an AI coach's steps give the instance's MCP address, the key and a ready command", async () => {
  const server = connectServer()
  renderAt('/settings/devices?connect=coach')

  const sheet = within(await screen.findByRole('dialog', { name: 'Connect' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Create the access key' }))

  // The address is the server's own, its base URL, whatever the page's.
  const address = await sheet.findByRole('button', { name: "Copy: The server's address (MCP)" })
  await waitFor(() => {
    expect(document.activeElement).toBe(address)
  })
  expect(valueOf("The server's address (MCP)")).toBe('https://tdeemon.example.com/mcp')
  expect(valueOf('The access key, shown only now')).toBe('tdm_made_up')
  expect(valueOf('Claude Code: run this in a terminal')).toBe(
    'claude mcp add --transport http tdeemon https://tdeemon.example.com/mcp --header "Authorization: Bearer tdm_made_up"',
  )
  const desktop: unknown = JSON.parse(
    valueOf(
      'Claude Desktop: add this to its claude_desktop_config.json, through mcp-remote, and restart it.',
    ) ?? '',
  )
  expect(desktop).toEqual({
    mcpServers: {
      tdeemon: {
        command: 'npx',
        // ${AUTH_HEADER} is mcp-remote's, not a template's.
        args: [
          'mcp-remote',
          'https://tdeemon.example.com/mcp',
          '--header',
          'Authorization:${AUTH_HEADER}',
        ],
        env: { AUTH_HEADER: 'Bearer tdm_made_up' },
      },
    },
  })
  expect(sheet.getByText(/^ChatGPT signs in to a server with OAuth, not a key/)).toBeDefined()
  expect(valueOf('Then write to your AI coach')).toBe('Look at my data and propose a plan.')
  expect(server.sent).toEqual([{ name: 'Claude', scopes: ['tools'] }])
})

test('the connection waits for the first call, and says it came without a reload', async () => {
  const server = connectServer()
  renderAt('/settings/devices?connect=coach')

  const sheet = within(await screen.findByRole('dialog', { name: 'Connect' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Create the access key' }))
  const state = await sheet.findByText('Waiting for the first call…')
  expect(state.tagName).toBe('OUTPUT')

  // The client calls, five seconds ago; the sheet asks again by itself.
  server.called(new Date(Date.now() - 5000).toISOString())
  expect(await sheet.findByText('Connected', {}, { timeout: 5000 })).toBe(state)
  expect(sheet.getByText(/^· last call \d+ sec\. ago$/)).toBeDefined()
})

test("a phone's steps are Health Auto Export's checklist, with its address and header", async () => {
  const server = connectServer()
  renderAt('/settings/devices?connect=phone')

  const sheet = within(await screen.findByRole('dialog', { name: 'Connect' }))
  expect(sheet.getByLabelText('Name')).toHaveProperty('value', 'iPhone')
  fireEvent.click(sheet.getByRole('button', { name: 'Create the access key' }))

  expect(await sheet.findByText('Method POST, format JSON')).toBeDefined()
  expect(valueOf('URL')).toBe('https://tdeemon.example.com/api/v1/ingest/hae')
  expect(valueOf('Header Authorization, with this value')).toBe('Bearer tdm_made_up')
  expect(sheet.getByText(/^Metrics: step count, active energy/)).toBeDefined()
  expect(sheet.getByText('Summarize Data: on, with the time grouping by day')).toBeDefined()
  expect(sheet.getByText('Waiting for the first call…')).toBeDefined()
  expect(server.sent).toEqual([{ name: 'iPhone', scopes: ['ingest'] }])
})

test("without the server's word on its address, the page's is given", async () => {
  connectServer(() => problem(500))
  renderAt('/settings/devices?connect=coach')

  const sheet = within(await screen.findByRole('dialog', { name: 'Connect' }))
  fireEvent.click(sheet.getByRole('button', { name: 'Create the access key' }))
  await sheet.findByRole('button', { name: "Copy: The server's address (MCP)" })
  expect(valueOf("The server's address (MCP)")).toBe(`${window.location.origin}/mcp`)
})

test('the way to connect speaks Croatian, its access keys too', async () => {
  connectServer(undefined, 'hr')
  renderAt('/settings/devices?connect=choose')

  const sheet = within(await screen.findByRole('dialog', { name: 'Poveži' }))
  expect(sheet.getByRole('heading', { name: 'Što povezuješ?' })).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: /^AI trener/ }))
  expect(sheet.getByText('Čita i upisuje')).toBeDefined()
  fireEvent.click(sheet.getByRole('button', { name: 'Napravi pristupni ključ' }))
  expect(await sheet.findByText('Čeka prvi poziv…')).toBeDefined()
  expect(valueOf('Zatim napiši AI treneru')).toBe('Pogledaj moje podatke i predloži mi plan.')
})
