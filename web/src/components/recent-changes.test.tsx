import { cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import { setLocale } from '@/i18n/locale'
import { fakeServer, me, problem, renderAt, signedIn, today, type Handler } from '@/test/app'

type Change = components['schemas']['ChangeBody']

beforeEach(() => {
  // The clock only: an hour after the changes, in Los Angeles' afternoon.
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(`${today}T21:00:00Z`))
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

const deleted = '0193c3a5-0000-7000-8000-0000000000a1'

// change is a change of the user's, made through the app at 13:14 today
// on the user's clock; over is laid over it.
function change(over: Partial<Change>): Change {
  return {
    id: crypto.randomUUID(),
    action: 'meal.create',
    entity: '0193c3a5-0000-7000-8000-0000000000a9',
    at: `${today}T20:14:00Z`,
    client: 'web',
    days: [today],
    items: [],
    ...over,
  }
}

// The latest changes: a meal deleted, which undo takes back next, and
// the others before it, of each kind and client.
const deletion = change({
  action: 'meal.delete',
  entity: deleted,
  name: 'Skyr bowl',
  items: ['Skyr'],
  next: true,
})
const logged = change({
  action: 'meal.create',
  client: 'mcp',
  model: 'claude-opus-5-5',
  name: 'Oats, a banana, milk and honey with cinnamon on top',
  items: ['Oats', 'Banana', 'Milk'],
})
const latest: Change[] = [
  deletion,
  logged,
  change({ action: 'metric.set', entity: `${today}/weight`, client: 'api', metric: 'weight' }),
  change({ action: 'food.archive', name: 'Skyr', brand: 'Dukat', archived: false, days: [] }),
  change({ action: 'recipe.save', name: 'Chicken curry', client: 'mcp', days: [] }),
  change({ action: 'proposal.status', status: 'applied', client: 'mcp', days: [] }),
  change({ action: 'phase.set', entity: today, days: [] }),
  change({ action: 'meal.replace', name: 'Lunch', undone: true, at: '2026-10-22T20:14:00Z' }),
]

// serve answers the changes as changes says them, and the undo with undo.
function serve(changes: () => Change[], undo: Handler) {
  const undos: unknown[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/changes': () => Response.json({ changes: changes() }),
    'POST /api/v1/undo': async (request) => {
      undos.push(await request.json())
      return undo(request)
    },
  })
  return undos
}

test('the settings list the recent changes, each said with when and by whom', async () => {
  serve(
    () => latest,
    () => problem(500),
  )
  renderAt('/settings')

  fireEvent.click(await screen.findByRole('button', { name: 'Recent changes' }))

  const sheet = within(await screen.findByRole('dialog', { name: 'Recent changes' }))
  const rows = await sheet.findAllByRole('listitem')
  expect(rows.map((r) => r.textContent)).toEqual([
    'Deleted: Skyr bowl1:14 PM · youUndo',
    'Logged: Oats + 21:14 PM · your AI coach',
    'Saved: Weight1:14 PM · a device',
    'In use again: Skyr · Dukat1:14 PM · you',
    'Saved: Chicken curry1:14 PM · your AI coach',
    'A proposal applied1:14 PM · your AI coach',
    'A phase set1:14 PM · you',
    'Changed: LunchOct 22, 1:14 PM · youUndone',
  ])
  // Undo is the next's alone, named by what it takes back.
  expect(sheet.getAllByRole('button', { name: /^Undo/ })).toHaveLength(1)
  expect(sheet.getByRole('button', { name: 'Undo: Deleted: Skyr bowl' })).toBeDefined()
})

test('the next change is taken back from the list, after its toast is gone, and the list follows', async () => {
  let undone = false
  const undos = serve(
    () =>
      undone
        ? [
            { ...deletion, undone: true, next: false },
            { ...logged, next: true },
          ]
        : latest,
    () => {
      undone = true
      return Response.json({
        undone: {
          action: 'meal.delete',
          entity: deleted,
          at: `${today}T20:14:00Z`,
          client: 'web',
          days: [today],
        },
      })
    },
  )
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Recent changes' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Recent changes' }))
  fireEvent.click(await sheet.findByRole('button', { name: 'Undo: Deleted: Skyr bowl' }))

  expect(await screen.findByText('Undone: a meal deleted.')).toBeDefined()
  // Named, so taken back only while it is the latest.
  expect(undos).toEqual([
    { id: expect.any(String) as unknown, last: { action: 'meal.delete', entity: deleted } },
  ])
  expect(await sheet.findByRole('button', { name: 'Undo: Logged: Oats + 2' })).toBeDefined()
  await waitFor(() => {
    expect(sheet.getAllByRole('listitem')[0]?.textContent).toBe(
      'Deleted: Skyr bowl1:14 PM · youUndone',
    )
  })
})

// The list may follow before the rest of the page came again: the row
// whose Undo was tapped is gone by then, and the toast says the undo all
// the same.
test('an undo is said though its row went before the page came again', async () => {
  let undone = false
  // What the page asks again after the undo waits until the list followed.
  const waiting: (() => void)[] = []
  fakeServer({
    ...signedIn(),
    'GET /api/v1/me': async () => {
      if (undone) {
        await new Promise<void>((resolve) => {
          waiting.push(resolve)
        })
      }
      return Response.json(me())
    },
    'GET /api/v1/changes': () =>
      Response.json({
        changes: undone
          ? [
              { ...deletion, undone: true, next: false },
              { ...logged, next: true },
            ]
          : latest,
      }),
    'POST /api/v1/undo': () => {
      undone = true
      return Response.json({
        undone: {
          action: 'meal.delete',
          entity: deleted,
          at: `${today}T20:14:00Z`,
          client: 'web',
          days: [today],
        },
      })
    },
  })
  renderAt('/settings')

  fireEvent.click(await screen.findByRole('button', { name: 'Recent changes' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Recent changes' }))
  fireEvent.click(await sheet.findByRole('button', { name: 'Undo: Deleted: Skyr bowl' }))
  expect(await sheet.findByRole('button', { name: 'Undo: Logged: Oats + 2' })).toBeDefined()
  for (const release of waiting) {
    release()
  }

  expect(await screen.findByText('Undone: a meal deleted.')).toBeDefined()
})

test('an undo another change came before says so, in Croatian too', async () => {
  fakeServer({
    ...signedIn('hr'),
    'GET /api/v1/changes': () => Response.json({ changes: latest }),
    'POST /api/v1/undo': () => problem(412),
  })
  renderAt('/settings')

  fireEvent.click(await screen.findByRole('button', { name: 'Zadnje promjene' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Zadnje promjene' }))
  expect((await sheet.findAllByRole('listitem')).map((r) => r.textContent)).toContain(
    'Upisano: Oats + 213:14 · AI trener',
  )
  fireEvent.click(sheet.getByRole('button', { name: 'Poništi: Obrisano: Skyr bowl' }))

  expect(
    await screen.findByText('U međuvremenu je došla druga promjena, pa ništa nije poništeno.'),
  ).toBeDefined()
})

test('with no change yet, the list says so', async () => {
  serve(
    () => [],
    () => problem(404),
  )
  renderAt('/')

  fireEvent.click(await screen.findByRole('button', { name: 'Recent changes' }))
  const sheet = within(await screen.findByRole('dialog', { name: 'Recent changes' }))
  expect(await sheet.findByText('No changes yet.')).toBeDefined()
})
