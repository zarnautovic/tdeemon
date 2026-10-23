import { focusManager } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import type { LimitsBody } from '@/components/plan/limit-fields'
import { setLocale } from '@/i18n/locale'
import { fakeServer, problem, renderAt, signedIn } from '@/test/app'
import { limitsBody } from '@/test/plan'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  setLocale('en')
  localStorage.clear()
})

type Profile = components['schemas']['ProfileBody']

// planServer keeps a profile and limits as the server does: a field left
// out of a profile is cleared, and a loosened limit needs confirm. It
// keeps what it was sent, by "METHOD /path".
function planServer(
  start: { profile?: Profile; limits?: LimitsBody } = {},
  over: Record<string, (r: Request) => Response | Promise<Response>> = {},
) {
  const sent: Record<string, unknown[]> = {}
  let profile: Profile = start.profile ?? { missing: ['sex', 'birth_date', 'height_cm'] }
  let limits = start.limits ?? limitsBody()
  let profileFails = false
  const fetch = fakeServer({
    ...signedIn(),
    'GET /api/v1/profile': () => (profileFails ? problem(503) : Response.json(profile)),
    'GET /api/v1/limits': () => Response.json(limits),
    'PUT /api/v1/profile': async (r) => {
      const body: unknown = await r.json()
      ;(sent['PUT /api/v1/profile'] ??= []).push(body)
      const given = typeof body === 'object' && body !== null ? body : {}
      const missing = (['sex', 'birth_date', 'height_cm'] as const).filter((k) => !(k in given))
      profile = {
        ...('sex' in given && (given.sex === 'male' || given.sex === 'female')
          ? { sex: given.sex }
          : {}),
        ...('birth_date' in given && typeof given.birth_date === 'string'
          ? { birth_date: given.birth_date }
          : {}),
        ...('height_cm' in given && typeof given.height_cm === 'number'
          ? { height_cm: given.height_cm }
          : {}),
        missing,
      }
      return Response.json(profile)
    },
    'PUT /api/v1/limits': async (r) => {
      const body: unknown = await r.json()
      ;(sent['PUT /api/v1/limits'] ??= []).push(body)
      const given = typeof body === 'object' && body !== null ? body : {}
      const changes: Record<string, number> = {}
      if ('limits' in given && typeof given.limits === 'object' && given.limits !== null) {
        for (const [name, v] of Object.entries(given.limits)) {
          if (typeof v === 'number') {
            changes[name] = v
          }
        }
      }
      const confirm = 'confirm' in given && given.confirm === true
      const loosens = Object.entries(changes).some(([name, v]) => {
        const current = limits.limits[name] ?? 0
        return limits.looser_below.includes(name) ? v < current : v > current
      })
      if (loosens && !confirm) {
        return Response.json(
          { status: 409, title: 'Conflict', detail: 'loosening needs confirm: true' },
          { status: 409, headers: { 'Content-Type': 'application/problem+json' } },
        )
      }
      const next = { ...limits.limits, ...changes }
      limits = { ...limits, limits: next, effective: next }
      return Response.json(limits)
    },
    ...over,
  })
  return {
    fetch,
    sent,
    setLimits: (l: LimitsBody) => (limits = l),
    setProfile: (p: Profile) => (profile = p),
    failProfile: (fails: boolean) => (profileFails = fails),
  }
}

// limit is the field of a limit, by its name.
function limit(name: string): HTMLInputElement {
  const field = screen
    .getByText(name, { selector: 'span' })
    .closest('label')
    ?.querySelector('input')
  if (!field) {
    throw new Error(`no field ${name}`)
  }
  return field
}

// page is the page's own content, without the sections' links.
function page(): HTMLElement {
  const section = screen.getByRole('heading', { level: 1 }).closest('section')
  if (!section) {
    throw new Error('no page')
  }
  return section
}

const type = (field: HTMLElement, value: string) => {
  fireEvent.change(field, { target: { value } })
}

const save = (form: string) => {
  fireEvent.click(
    within(screen.getByRole('form', { name: form })).getByRole('button', { name: 'Save' }),
  )
}

test('the settings lead to the profile and limits of the plan', async () => {
  planServer()
  const router = renderAt('/settings')
  fireEvent.click(await screen.findByRole('link', { name: /Profile and limits of the plan/ }))
  await screen.findByRole('heading', { name: 'Profile and limits', level: 1 })
  expect(router.state.location.pathname).toBe('/settings/plan')
  fireEvent.click(within(page()).getByRole('link', { name: 'Settings' }))
  await screen.findByRole('heading', { name: 'Settings', level: 1 })
})

test('the profile says what a plan still needs, and is saved whole', async () => {
  const { sent } = planServer({
    profile: { sex: 'female', height_cm: 168.5, missing: ['birth_date'] },
  })
  renderAt('/settings/plan')
  const form = await screen.findByRole('form', { name: 'Profile' })
  expect(within(form).getByText('A plan still needs: birth date.')).toBeTruthy()
  expect(within(form).getByRole('radio', { name: 'Female' })).toHaveProperty('checked', true)
  const height = within(form).getByRole('textbox', { name: /Height/ })
  expect(height).toHaveProperty('value', '168.5')

  fireEvent.change(within(form).getByLabelText('Birth date'), { target: { value: '1990-06-15' } })
  type(height, '170,5')
  save('Profile')
  await within(form).findByText('A plan has all it needs of you.')
  expect(sent['PUT /api/v1/profile']).toEqual([
    { sex: 'female', birth_date: '1990-06-15', height_cm: 170.5 },
  ])

  // A field left empty is cleared, and the server says so.
  fireEvent.click(within(form).getByRole('radio', { name: 'Not given' }))
  type(height, '')
  save('Profile')
  await within(form).findByText('A plan still needs: sex, height.')
  expect(sent['PUT /api/v1/profile']?.[1]).toEqual({ birth_date: '1990-06-15' })
})

test("the profile is the server's, not one a cache kept from before", async () => {
  const { setProfile } = planServer({
    profile: { sex: 'female', missing: ['birth_date', 'height_cm'] },
  })
  renderAt('/settings/plan')
  expect(await screen.findByRole('radio', { name: 'Female' })).toHaveProperty('checked', true)
  fireEvent.click(within(page()).getByRole('link', { name: 'Settings' }))
  await screen.findByRole('heading', { name: 'Settings', level: 1 })
  // Changed in another tab meanwhile.
  setProfile({ sex: 'male', missing: ['birth_date', 'height_cm'] })
  fireEvent.click(screen.getByRole('link', { name: /Profile and limits of the plan/ }))
  await waitFor(() => {
    expect(screen.getByRole('radio', { name: 'Male' })).toHaveProperty('checked', true)
  })
})

test('a profile typed stays when it cannot be asked again', async () => {
  const { fetch, failProfile } = planServer()
  renderAt('/settings/plan')
  const form = await screen.findByRole('form', { name: 'Profile' })
  type(within(form).getByRole('textbox', { name: /Height/ }), '181')
  // The page comes back to the front while the server is down: the
  // profile is asked again, and fails.
  const failed = () =>
    fetch.mock.calls.filter(([input]) =>
      String(input instanceof Request ? input.url : input).endsWith('/api/v1/profile'),
    ).length
  const before = failed()
  failProfile(true)
  focusManager.setFocused(false)
  focusManager.setFocused(true)
  await waitFor(() => {
    expect(failed()).toBeGreaterThan(before)
  })
  await act(async () => {
    await fetch.mock.results.at(-1)?.value
  })
  expect(screen.getByRole('textbox', { name: /Height/ })).toHaveProperty('value', '181')
  failProfile(false)
  focusManager.setFocused(undefined)
  expect(screen.getByRole('textbox', { name: /Height/ })).toHaveProperty('value', '181')
})

test('a height that is no number, or none the server takes, is not sent', async () => {
  const { sent } = planServer()
  renderAt('/settings/plan')
  const form = await screen.findByRole('form', { name: 'Profile' })
  const height = within(form).getByRole('textbox', { name: /Height/ })
  const refused = async (typed: string) => {
    type(height, typed)
    save('Profile')
    expect(await within(form).findByRole('alert')).toHaveProperty(
      'textContent',
      'The height is 50 to 275 cm.',
    )
  }
  await refused('tall')
  // In metres, and past the tallest.
  await refused('1,8')
  await refused('300')
  expect(height.getAttribute('aria-invalid')).toBe('true')
  expect(sent['PUT /api/v1/profile']).toBeUndefined()
})

test('a height is shown as it is stored, not rounded', async () => {
  const { sent } = planServer({ profile: { height_cm: 168.25, missing: ['sex', 'birth_date'] } })
  renderAt('/settings/plan')
  const form = await screen.findByRole('form', { name: 'Profile' })
  expect(within(form).getByRole('textbox', { name: /Height/ })).toHaveProperty('value', '168.25')
  fireEvent.click(within(form).getByRole('radio', { name: 'Male' }))
  save('Profile')
  await waitFor(() => {
    expect(sent['PUT /api/v1/profile']).toEqual([{ sex: 'male', height_cm: 168.25 }])
  })
})

test('each limit is shown against its default and the instance', async () => {
  const body = limitsBody({ cut_rate: -0.5, step_kcal: 300 })
  // The instance holds the step at its own, 200 kcal.
  body.effective = { ...body.limits, step_kcal: 200 }
  planServer({ limits: body })
  renderAt('/settings/plan')
  const cut = await screen.findByRole('textbox', { name: /^Cut, fastest loss, BMI below 30/ })
  expect(cut).toHaveProperty('value', '0.5')
  expect(cut.getAttribute('aria-describedby')).toBeTruthy()
  const hint = document.getElementById(cut.getAttribute('aria-describedby') ?? '')
  expect(hint?.textContent).toBe("Default 0.7 · the instance's 1")
  const step = limit('Autopilot step, most')
  expect(document.getElementById(step.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    "Default 100 · the instance's 200 · Holds: 200, the instance's",
  )
  // The grams' floor is the user's only.
  expect(
    document.getElementById(
      limit('Protein, least in grams (0 for none)').getAttribute('aria-describedby') ?? '',
    )?.textContent,
  ).toBe('Default 0')
})

test('a tightened limit is saved as it is, and only what changed is sent', async () => {
  const { sent } = planServer()
  renderAt('/settings/plan')
  type(await screen.findByRole('textbox', { name: /^Cut, fastest loss, BMI below 30/ }), '0,5')
  type(limit('Kcal of a day, least, male'), '1600')
  save('Limits of the plan')
  await screen.findByText('Saved')
  expect(sent['PUT /api/v1/limits']).toEqual([
    { limits: { cut_rate: -0.5, kcal_floor_male: 1600 } },
  ])
  expect(limit('Cut, fastest loss, BMI below 30')).toHaveProperty('value', '0.5')
})

test('loosening a limit asks first, and is sent with the confirmation', async () => {
  const { sent } = planServer()
  renderAt('/settings/plan')
  type(await screen.findByRole('textbox', { name: /^Bulk, fastest gain/ }), '0.4')
  type(limit('Protein, least'), '1.8')
  save('Limits of the plan')
  const asked = await screen.findByText(
    'Loosening Bulk, fastest gain allows a plan the limit refused. Loosen it?',
  )
  expect(sent['PUT /api/v1/limits']).toBeUndefined()

  // Back sends nothing, and keeps what was typed.
  fireEvent.click(screen.getByRole('button', { name: 'Back' }))
  expect(asked.isConnected).toBe(false)
  expect(limit('Bulk, fastest gain')).toHaveProperty('value', '0.4')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Loosen' }))
  await screen.findByText('Saved')
  expect(sent['PUT /api/v1/limits']).toEqual([
    { limits: { bulk_rate: 0.4, protein_floor: 1.8 }, confirm: true },
  ])
})

test('a loosening the server finds, the limits changed elsewhere, is asked of the user', async () => {
  const { sent, setLimits } = planServer()
  renderAt('/settings/plan')
  const bulk = await screen.findByRole('textbox', { name: /^Bulk, fastest gain/ })
  // Another tab tightened the bulk meanwhile, so 0.2 loosens it now.
  setLimits(limitsBody({ bulk_rate: 0.1 }))
  type(bulk, '0.2')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Loosen' }))
  await screen.findByText('Saved')
  expect(sent['PUT /api/v1/limits']).toEqual([
    { limits: { bulk_rate: 0.2 } },
    { limits: { bulk_rate: 0.2 }, confirm: true },
  ])
})

test('a limit past the instance is not sent, and says how far it may go', async () => {
  const { sent } = planServer()
  renderAt('/settings/plan')
  const cut = await screen.findByRole('textbox', { name: /^Cut, fastest loss, BMI below 30/ })
  type(cut, '1.2')
  expect(cut.getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByText("Past the instance's limit, 1.")).toBeTruthy()
  save('Limits of the plan')
  expect(await screen.findByText('Some limits cannot be saved as they are.')).toBeTruthy()
  type(limit('Logged days a step needs'), '3.5')
  expect(screen.getByText('A whole number of days.')).toBeTruthy()
  expect(sent['PUT /api/v1/limits']).toBeUndefined()
})

test("a limit past the instance's other one says which to lower", async () => {
  const body = limitsBody({ protein_floor: 2.8 })
  body.instance.protein_ceiling = 2.5
  const { sent } = planServer({ limits: body })
  renderAt('/settings/plan')
  expect(
    await screen.findByText(
      "Past the instance's Protein, most, 2.5: lower it to 2.5 at most, or no change of your limits is taken.",
    ),
  ).toBeTruthy()
  type(limit('Autopilot step, most'), '80')
  save('Limits of the plan')
  await screen.findByText('Some limits cannot be saved as they are.')
  // Lowering a floor loosens it, which the user confirms.
  type(limit('Protein, least'), '2.5')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Loosen' }))
  await screen.findByText('Saved')
  expect(sent['PUT /api/v1/limits']).toEqual([
    { limits: { protein_floor: 2.5, step_kcal: 80 }, confirm: true },
  ])
})

test('a limit outside what any level may have says so, and is not sent', async () => {
  const { sent } = planServer()
  renderAt('/settings/plan')
  type(await screen.findByRole('textbox', { name: /^Logged days a step needs/ }), '8')
  expect(screen.getByText('From 0 to 7.')).toBeTruthy()
  type(limit('Fat, least'), '120')
  expect(screen.getByText('From 0 to 100.')).toBeTruthy()
  save('Limits of the plan')
  await screen.findByText('Some limits cannot be saved as they are.')
  expect(sent['PUT /api/v1/limits']).toBeUndefined()
})

test('the question to loosen takes the focus, and a change of a field closes it', async () => {
  planServer()
  renderAt('/settings/plan')
  type(await screen.findByRole('textbox', { name: /^Bulk, fastest gain/ }), '0.4')
  save('Limits of the plan')
  const back = await screen.findByRole('button', { name: 'Back' })
  expect(document.activeElement).toBe(back)
  expect(document.getElementById(back.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Loosening Bulk, fastest gain allows a plan the limit refused. Loosen it?',
  )
  type(limit('Bulk, fastest gain'), '0.45')
  expect(screen.queryByRole('button', { name: 'Back' })).toBeNull()
})

test('a loosening on its way cannot be taken back', async () => {
  planServer({}, { 'PUT /api/v1/limits': () => new Promise<Response>(() => undefined) })
  renderAt('/settings/plan')
  type(await screen.findByRole('textbox', { name: /^Bulk, fastest gain/ }), '0.4')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Loosen' }))
  await waitFor(() => {
    expect(screen.getByRole('button', { name: 'Back' }).matches(':disabled')).toBe(true)
  })
})

test('a loosening confirmed and refused says why, the question answered', async () => {
  planServer(
    {},
    {
      'PUT /api/v1/limits': () =>
        Response.json(
          { status: 422, title: 'Unprocessable Entity', detail: 'bulk_rate 0.4: want less' },
          { status: 422, headers: { 'Content-Type': 'application/problem+json' } },
        ),
    },
  )
  renderAt('/settings/plan')
  type(await screen.findByRole('textbox', { name: /^Bulk, fastest gain/ }), '0.4')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Loosen' }))
  expect(
    await screen.findByText('The server did not take it: "Bulk, fastest gain" 0.4: want less'),
  ).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Loosen' })).toBeNull()
})

test('back from a loosening the server found leaves no word of it', async () => {
  const { setLimits } = planServer()
  renderAt('/settings/plan')
  const bulk = await screen.findByRole('textbox', { name: /^Bulk, fastest gain/ })
  setLimits(limitsBody({ bulk_rate: 0.1 }))
  type(bulk, '0.2')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Back' }))
  expect(screen.queryByText(/needs confirm/)).toBeNull()
  expect(screen.queryByRole('alert')).toBeNull()
})

test('the limits hold still while they are saved, and a failure goes once they change', async () => {
  let fail = true
  let answer: ((r: Response) => void) | undefined
  planServer(
    {},
    {
      'PUT /api/v1/limits': () =>
        fail
          ? problem(500)
          : new Promise<Response>((resolve) => {
              answer = resolve
            }),
    },
  )
  renderAt('/settings/plan')
  const cut = await screen.findByRole('textbox', { name: /^Cut, fastest loss, BMI below 30/ })
  type(cut, '0.5')
  save('Limits of the plan')
  expect(await screen.findByRole('button', { name: 'Try again' })).toBeTruthy()
  // Typed again, what failed is no longer what would be sent again.
  type(cut, '0.6')
  expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()

  fail = false
  save('Limits of the plan')
  await waitFor(() => {
    expect(cut.matches(':disabled')).toBe(true)
  })
  answer?.(Response.json(limitsBody({ cut_rate: -0.6 })))
  await waitFor(() => {
    expect(cut.matches(':disabled')).toBe(false)
  })
})

test("the server's refusal names the limits as the page does", async () => {
  planServer(
    {},
    {
      'PUT /api/v1/limits': () =>
        Response.json(
          {
            status: 422,
            title: 'Unprocessable Entity',
            detail: 'drift_pct 120: want 0 to 100 %',
          },
          { status: 422, headers: { 'Content-Type': 'application/problem+json' } },
        ),
    },
  )
  renderAt('/settings/plan')
  type(await screen.findByRole('textbox', { name: /^Drift from/ }), '12')
  save('Limits of the plan')
  expect(
    await screen.findByText(
      'The server did not take it: "Drift from the target you agreed to, most" 120: want 0 to 100 %',
    ),
  ).toBeTruthy()
})

test('in Croatian a limit is typed with a decimal comma', async () => {
  const { sent } = planServer({}, signedIn('hr'))
  renderAt('/settings/plan')
  const cut = await screen.findByRole('textbox', { name: /^Cut, najbrži gubitak, BMI ispod 30/ })
  expect(cut).toHaveProperty('value', '0,7')
  type(cut, '0,6')
  fireEvent.click(
    within(screen.getByRole('form', { name: 'Granice plana' })).getByRole('button', {
      name: 'Spremi',
    }),
  )
  await waitFor(() => {
    expect(sent['PUT /api/v1/limits']).toEqual([{ limits: { cut_rate: -0.6 } }])
  })
})
