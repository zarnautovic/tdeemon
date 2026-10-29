import { focusManager } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import type { components } from '@/api/schema'
import type { LimitsBody } from '@/components/plan/limit-fields'
import { setLocale } from '@/i18n/locale'
import { fakeServer, me, problem, renderAt, signedIn } from '@/test/app'
import { limitsBody, planBody } from '@/test/plan'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
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
    'GET /api/v1/me': () =>
      Response.json({
        ...me(),
        // The age on the tests' today, of a birth date in 1990 after
        // October 23.
        profile: { ...profile, ...(profile.birth_date === undefined ? {} : { age: 36 }) },
      }),
    'GET /api/v1/profile': () => (profileFails ? problem(503) : Response.json(profile)),
    'GET /api/v1/limits': () => Response.json(limits),
    'GET /api/v1/plan': () => Response.json(planBody()),
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

// group is the button that opens or closes a group of the limits.
function group(name: string): HTMLElement {
  return screen.getByRole('button', { name: new RegExp(`^${name}`) })
}

// main is the page's own content, without the sections' links.
function main(): HTMLElement {
  return screen.getByRole('main')
}

// summaryOf is what a row of a list says of its value.
function summaryOf(row: HTMLElement): string | null | undefined {
  return document.getElementById(row.getAttribute('aria-describedby') ?? '')?.textContent
}

const type = (field: HTMLElement, value: string) => {
  fireEvent.change(field, { target: { value } })
}

const save = (form: string) => {
  fireEvent.click(
    within(screen.getByRole('form', { name: form })).getByRole('button', { name: 'Save' }),
  )
}

const cutLabel = 'Fastest loss in a cut, BMI under 30'

test("the plan lists what it rests on of the user's, each a row to its page, and back", async () => {
  planServer({
    profile: { sex: 'female', birth_date: '1990-06-15', height_cm: 168, missing: [] },
    limits: limitsBody({ cut_rate: -0.5 }),
  })
  const router = renderAt('/plan')
  await screen.findByRole('heading', { name: 'Plan', level: 1 })
  const you = within(screen.getByRole('region', { name: 'You and limits' }))
  const profile = you.getByRole('link', { name: 'Profile' })
  await waitFor(() => {
    expect(summaryOf(profile)).toBe('Female · 36 years · 168 cm')
  })
  await waitFor(() => {
    expect(summaryOf(you.getByRole('link', { name: 'Limits of the plan' }))).toBe(
      'Loss up to 0.5% a week · protein from 1.6 g/kg',
    )
  })
  expect(summaryOf(you.getByRole('link', { name: 'Notes for your AI coach' }))).toBe('None yet')
  // A phone has the settings there too.
  const settings = within(screen.getByRole('region', { name: 'Settings' }))
  expect(settings.getByRole('link', { name: 'Appearance' })).toBeDefined()
  expect(settings.getByRole('button', { name: 'Sign out' })).toBeDefined()

  fireEvent.click(profile)
  await screen.findByRole('heading', { name: 'Profile', level: 1 })
  expect(router.state.location.pathname).toBe('/plan/profile')
  fireEvent.click(within(main()).getByRole('link', { name: 'Plan' }))
  await screen.findByRole('heading', { name: 'Plan', level: 1 })
  expect(router.state.location.pathname).toBe('/plan')
})

test('a profile a plan still needs some of says what, as a warning', async () => {
  planServer({ profile: { sex: 'female', missing: ['birth_date', 'height_cm'] } })
  renderAt('/plan')
  const row = await screen.findByRole('link', { name: 'Profile' })
  await waitFor(() => {
    expect(summaryOf(row)).toBe('A plan still needs: birth date, height.')
  })
  expect(document.getElementById(row.getAttribute('aria-describedby') ?? '')?.className).toContain(
    'text-warning',
  )
})

test('the profile says what a plan still needs, and is saved whole', async () => {
  const { sent } = planServer({
    profile: { sex: 'female', height_cm: 168.5, missing: ['birth_date'] },
  })
  renderAt('/plan/profile')
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

test('a profile saved is the one the list and the page have back', async () => {
  planServer()
  renderAt('/plan/profile')
  const form = await screen.findByRole('form', { name: 'Profile' })
  fireEvent.click(within(form).getByRole('radio', { name: 'Male' }))
  fireEvent.change(within(form).getByLabelText('Birth date'), { target: { value: '1990-11-02' } })
  type(within(form).getByRole('textbox', { name: /Height/ }), '181')
  save('Profile')
  await within(form).findByText('A plan has all it needs of you.')
  expect(await screen.findByText('Saved')).toBeDefined()

  fireEvent.click(within(main()).getByRole('link', { name: 'Plan' }))
  const row = await screen.findByRole('link', { name: 'Profile' })
  await waitFor(() => {
    expect(summaryOf(row)).toBe('Male · 36 years · 181 cm')
  })
  fireEvent.click(row)
  const again = await screen.findByRole('form', { name: 'Profile' })
  expect(within(again).getByRole('radio', { name: 'Male' })).toHaveProperty('checked', true)
  expect(within(again).getByLabelText('Birth date')).toHaveProperty('value', '1990-11-02')
  expect(within(again).getByRole('textbox', { name: /Height/ })).toHaveProperty('value', '181')
})

test("the profile is the server's, not one a cache kept from before", async () => {
  const { setProfile } = planServer({
    profile: { sex: 'female', missing: ['birth_date', 'height_cm'] },
  })
  renderAt('/plan/profile')
  expect(await screen.findByRole('radio', { name: 'Female' })).toHaveProperty('checked', true)
  fireEvent.click(within(main()).getByRole('link', { name: 'Plan' }))
  await screen.findByRole('heading', { name: 'Plan', level: 1 })
  // Changed in another tab meanwhile.
  setProfile({ sex: 'male', missing: ['birth_date', 'height_cm'] })
  fireEvent.click(screen.getByRole('link', { name: 'Profile' }))
  await waitFor(() => {
    expect(screen.getByRole('radio', { name: 'Male' })).toHaveProperty('checked', true)
  })
})

test('a profile typed stays when it cannot be asked again', async () => {
  const { fetch, failProfile } = planServer()
  renderAt('/plan/profile')
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
  expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
  failProfile(false)
  focusManager.setFocused(undefined)
  expect(screen.getByRole('textbox', { name: /Height/ })).toHaveProperty('value', '181')
})

test('a profile that did not come says why, and comes when asked again', async () => {
  const { failProfile } = planServer()
  failProfile(true)
  renderAt('/plan/profile')
  const again = await screen.findByRole('button', { name: 'Try again' })
  expect(screen.queryByRole('form', { name: 'Profile' })).toBeNull()
  failProfile(false)
  fireEvent.click(again)
  expect(await screen.findByRole('form', { name: 'Profile' })).toBeTruthy()
  expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
})

test('limits that did not come say why, and come when asked again', async () => {
  let fails = true
  planServer(
    {},
    { 'GET /api/v1/limits': () => (fails ? problem(503) : Response.json(limitsBody())) },
  )
  renderAt('/plan/limits')
  const again = await screen.findByRole('button', { name: 'Try again' })
  expect(screen.queryByRole('form', { name: 'Limits of the plan' })).toBeNull()
  fails = false
  fireEvent.click(again)
  expect(await screen.findByRole('form', { name: 'Limits of the plan' })).toBeTruthy()
})

test('a cached profile asked again says it is loading, in the shape of the form', async () => {
  let hang = false
  planServer(
    {},
    {
      'GET /api/v1/profile': () =>
        hang
          ? new Promise<Response>(() => undefined)
          : Response.json({ missing: ['sex', 'birth_date', 'height_cm'] }),
    },
  )
  renderAt('/plan/profile')
  await screen.findByRole('form', { name: 'Profile' })
  fireEvent.click(within(main()).getByRole('link', { name: 'Plan' }))
  await screen.findByRole('heading', { name: 'Plan', level: 1 })
  hang = true
  fireEvent.click(screen.getByRole('link', { name: 'Profile' }))
  await screen.findByRole('heading', { name: 'Profile', level: 1 })
  expect(screen.getByText('Loading…')).toBeTruthy()
  expect(screen.queryByRole('form', { name: 'Profile' })).toBeNull()
})

test('a birth date typed in part is not sent, as it would clear the one stored', async () => {
  const { sent } = planServer({
    profile: { birth_date: '1990-06-15', missing: ['sex', 'height_cm'] },
  })
  renderAt('/plan/profile')
  const form = await screen.findByRole('form', { name: 'Profile' })
  const date = within(form).getByLabelText('Birth date')
  // The browser's word for a date part typed: jsdom has none of its own.
  Object.defineProperty(date, 'validity', { value: { badInput: true }, configurable: true })
  fireEvent.change(date, { target: { value: '' } })
  save('Profile')
  expect(await within(form).findByRole('alert')).toHaveProperty(
    'textContent',
    'The birth date is not a whole date: give it all, or clear it.',
  )
  expect(date.getAttribute('aria-invalid')).toBe('true')
  expect(sent['PUT /api/v1/profile']).toBeUndefined()
})

test('a birth date after today, or of an age past the oldest, is not sent', async () => {
  const { sent } = planServer({
    profile: { birth_date: '1990-06-15', missing: ['sex', 'height_cm'] },
  })
  renderAt('/plan/profile')
  const form = await screen.findByRole('form', { name: 'Profile' })
  const date = within(form).getByLabelText('Birth date')

  // Today is October 23, 2026.
  fireEvent.change(date, { target: { value: '2026-10-24' } })
  save('Profile')
  expect((await within(form).findByRole('alert')).textContent).toBe(
    'The birth date is after today.',
  )
  // 131 on the 23rd, a day short of 132.
  fireEvent.change(date, { target: { value: '1894-10-24' } })
  save('Profile')
  expect(within(form).getByRole('alert').textContent).toBe(
    'The birth date makes an age of more than 130.',
  )
  expect(date.getAttribute('aria-invalid')).toBe('true')
  expect(sent['PUT /api/v1/profile']).toBeUndefined()
  // 130 on the 23rd, as the server counts it: sent.
  fireEvent.change(date, { target: { value: '1895-10-24' } })
  save('Profile')
  await waitFor(() => {
    expect(sent['PUT /api/v1/profile']).toHaveLength(1)
  })
})

test('a height that is no number, or none the server takes, is not sent', async () => {
  const { sent } = planServer()
  renderAt('/plan/profile')
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
  renderAt('/plan/profile')
  const form = await screen.findByRole('form', { name: 'Profile' })
  expect(within(form).getByRole('textbox', { name: /Height/ })).toHaveProperty('value', '168.25')
  fireEvent.click(within(form).getByRole('radio', { name: 'Male' }))
  save('Profile')
  await waitFor(() => {
    expect(sent['PUT /api/v1/profile']).toEqual([{ sex: 'male', height_cm: 168.25 }])
  })
})

test('the limits come in groups, the pace open, each saying what of it changed', async () => {
  planServer({ limits: limitsBody({ protein_floor: 1.8, fat_floor_pct: 25 }) })
  renderAt('/plan/limits')
  await screen.findByRole('form', { name: 'Limits of the plan' })
  const pace = group('Pace')
  expect(pace.getAttribute('aria-expanded')).toBe('true')
  expect(pace.textContent).toContain('As by default')
  expect(screen.getByRole('textbox', { name: new RegExp(`^${cutLabel}`) })).toBeTruthy()
  const food = group('Food')
  expect(food.getAttribute('aria-expanded')).toBe('false')
  expect(food.textContent).toContain('2 changed')
  // A closed group's fields are not shown, until it opens.
  expect(screen.queryByRole('textbox', { name: /^Least protein(?! in)/ })).toBeNull()
  fireEvent.click(food)
  expect(food.getAttribute('aria-expanded')).toBe('true')
  expect(await screen.findByRole('textbox', { name: /^Least protein(?! in)/ })).toHaveProperty(
    'value',
    '1.8',
  )
})

test('each limit is shown against its default and the instance', async () => {
  const body = limitsBody({ cut_rate: -0.5, step_kcal: 300 })
  // The instance holds the step at its own, 200 kcal.
  body.effective = { ...body.limits, step_kcal: 200 }
  planServer({ limits: body })
  renderAt('/plan/limits')
  const cut = await screen.findByRole('textbox', { name: new RegExp(`^${cutLabel}`) })
  expect(cut).toHaveProperty('value', '0.5')
  expect(cut.getAttribute('aria-describedby')).toBeTruthy()
  const hint = document.getElementById(cut.getAttribute('aria-describedby') ?? '')
  expect(hint?.textContent).toBe('Default 0.7 · instance limit 1')
  const step = limit('Largest step of the autopilot')
  expect(document.getElementById(step.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    "Default 100 · instance limit 200 · Holds: 200, the instance's",
  )
  // The grams' floor is the user's only.
  expect(
    document.getElementById(
      limit('Least protein in grams (0 for none)').getAttribute('aria-describedby') ?? '',
    )?.textContent,
  ).toBe('Default 0')
})

// unitOf is the unit shown beside a limit's field.
const unitOf = (field: HTMLInputElement) => field.nextElementSibling?.textContent

test('a unit of days agrees with the number typed beside it', async () => {
  planServer()
  renderAt('/plan/limits')
  await screen.findByRole('form', { name: 'Limits of the plan' })
  fireEvent.click(group('Autopilot'))
  const between = limit('Days between two steps, at least')
  expect(unitOf(between)).toBe('days')
  type(between, '1')
  expect(unitOf(between)).toBe('day')
  // No number yet: the form of many.
  type(between, '')
  expect(unitOf(between)).toBe('days')
  expect(unitOf(limit('Days before the first step of a phase'))).toBe('days')
})

// A percentage reads as the language writes it: right after the number
// in English (the plan's rows above), a space between them in Croatian,
// one that keeps them on a line.
test("in Croatian a limit's percentage has a space before its sign", async () => {
  planServer({ limits: limitsBody({ cut_rate: -0.5 }) }, signedIn('hr'))
  renderAt('/plan')
  const you = within(await screen.findByRole('region', { name: 'Ti i granice' }))
  await waitFor(() => {
    expect(summaryOf(you.getByRole('link', { name: 'Granice plana' }))).toBe(
      'Gubitak do 0,5 % tjedno · proteini od 1,6 g/kg',
    )
  })
})

test('in Croatian a unit of days agrees with its number too', async () => {
  planServer({ limits: limitsBody({ first_step_days: 21 }) }, signedIn('hr'))
  renderAt('/plan/limits')
  await screen.findByRole('form', { name: 'Granice plana' })
  fireEvent.click(group('Autopilot'))
  const between = limit('Dana između dva koraka, najmanje')
  expect(unitOf(between)).toBe('dana')
  type(between, '1')
  expect(unitOf(between)).toBe('dan')
  type(between, '3')
  expect(unitOf(between)).toBe('dana')
  expect(unitOf(limit('Dana prije prvog koraka faze'))).toBe('dan')
})

test('a tightened limit is saved as it is, and only what changed is sent', async () => {
  const { sent } = planServer()
  renderAt('/plan/limits')
  type(await screen.findByRole('textbox', { name: new RegExp(`^${cutLabel}`) }), '0,5')
  fireEvent.click(group('Food'))
  type(limit('Fewest kcal of a day, for a man'), '1600')
  save('Limits of the plan')
  await screen.findByText('Saved')
  expect(sent['PUT /api/v1/limits']).toEqual([
    { limits: { cut_rate: -0.5, kcal_floor_male: 1600 } },
  ])
  expect(limit(cutLabel)).toHaveProperty('value', '0.5')
  // The groups say what changed now, and the page opened again has it.
  expect(group('Pace').textContent).toContain('1 changed')
  fireEvent.click(within(main()).getByRole('link', { name: 'Plan' }))
  const row = await screen.findByRole('link', { name: 'Limits of the plan' })
  await waitFor(() => {
    expect(summaryOf(row)).toBe('Loss up to 0.5% a week · protein from 1.6 g/kg')
  })
  fireEvent.click(row)
  expect(await screen.findByRole('textbox', { name: new RegExp(`^${cutLabel}`) })).toHaveProperty(
    'value',
    '0.5',
  )
})

test('loosening a limit asks first, and is sent with the confirmation', async () => {
  const { sent } = planServer()
  renderAt('/plan/limits')
  type(await screen.findByRole('textbox', { name: /^Fastest gain in a bulk/ }), '0.4')
  fireEvent.click(group('Food'))
  type(limit('Least protein'), '1.8')
  save('Limits of the plan')
  const asked = await screen.findByText(
    'Loosening Fastest gain in a bulk allows a plan the limit refused. Loosen it?',
  )
  expect(sent['PUT /api/v1/limits']).toBeUndefined()

  // Back sends nothing, and keeps what was typed.
  fireEvent.click(screen.getByRole('button', { name: 'Back' }))
  expect(asked.isConnected).toBe(false)
  expect(limit('Fastest gain in a bulk')).toHaveProperty('value', '0.4')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Loosen' }))
  await screen.findByText('Saved')
  expect(sent['PUT /api/v1/limits']).toEqual([
    { limits: { bulk_rate: 0.4, protein_floor: 1.8 }, confirm: true },
  ])
})

test('a loosening the server finds, the limits changed elsewhere, is asked of the user', async () => {
  const { sent, setLimits } = planServer()
  renderAt('/plan/limits')
  const bulk = await screen.findByRole('textbox', { name: /^Fastest gain in a bulk/ })
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

// QA #215: a limit typed past the instance's, then saved, said so only at
// the foot of the form, many fields below the one that was wrong.
test('a wrong limit takes the focus and comes into view, and says why at its field', async () => {
  const { sent } = planServer()
  const scrolled = vi.spyOn(Element.prototype, 'scrollIntoView')
  renderAt('/plan/limits')
  const cut = await screen.findByRole('textbox', { name: new RegExp(`^${cutLabel}`) })
  type(cut, '2')
  // Moved away, then saved.
  limit('Fastest gain in a bulk').focus()
  save('Limits of the plan')
  expect(document.activeElement).toBe(cut)
  expect(scrolled).toHaveBeenCalledWith({ block: 'center' })
  expect(scrolled.mock.contexts.at(-1)).toBe(cut)
  expect(cut.getAttribute('aria-invalid')).toBe('true')
  const said = cut
    .getAttribute('aria-describedby')
    ?.split(' ')
    .map((id) => document.getElementById(id)?.textContent)
  expect(said).toEqual(['Default 0.7 · instance limit 1', "Past the instance's limit, 1."])
  expect(group('Pace').textContent).toContain('1 to fix')
  expect(sent['PUT /api/v1/limits']).toBeUndefined()
})

test('a wrong limit in a closed group opens it, and takes the focus', async () => {
  const { sent } = planServer()
  renderAt('/plan/limits')
  await screen.findByRole('form', { name: 'Limits of the plan' })
  const autopilot = group('Autopilot')
  fireEvent.click(autopilot)
  const logged = limit('Logged days a step needs')
  type(logged, '8')
  expect(screen.getByText('From 0 to 7.')).toBeTruthy()
  fireEvent.click(autopilot)
  expect(autopilot.getAttribute('aria-expanded')).toBe('false')
  // A change of the pace, valid, and the wrong one closed away.
  type(limit(cutLabel), '0.6')
  save('Limits of the plan')
  expect(autopilot.getAttribute('aria-expanded')).toBe('true')
  expect(document.activeElement).toBe(logged)
  expect(autopilot.textContent).toContain('1 to fix')
  expect(sent['PUT /api/v1/limits']).toBeUndefined()
})

test('a limit past the instance is not sent, and says how far it may go', async () => {
  const { sent } = planServer()
  renderAt('/plan/limits')
  const cut = await screen.findByRole('textbox', { name: new RegExp(`^${cutLabel}`) })
  type(cut, '1.2')
  expect(cut.getAttribute('aria-invalid')).toBe('true')
  expect(screen.getByText("Past the instance's limit, 1.")).toBeTruthy()
  fireEvent.click(group('Autopilot'))
  type(limit('Logged days a step needs'), '3.5')
  expect(screen.getByText('A whole number of days.')).toBeTruthy()
  save('Limits of the plan')
  // The first wrong one, in the form's order, has the focus.
  expect(document.activeElement).toBe(cut)
  expect(sent['PUT /api/v1/limits']).toBeUndefined()
})

test("a limit past the instance's other one says which to lower", async () => {
  const body = limitsBody({ protein_floor: 2.8 })
  body.instance.protein_ceiling = 2.5
  const { sent } = planServer({ limits: body })
  renderAt('/plan/limits')
  await screen.findByRole('form', { name: 'Limits of the plan' })
  fireEvent.click(group('Autopilot'))
  type(limit('Largest step of the autopilot'), '80')
  save('Limits of the plan')
  // Its group opens, and the field takes the focus.
  expect(group('Food').getAttribute('aria-expanded')).toBe('true')
  expect(document.activeElement).toBe(limit('Least protein'))
  expect(
    screen.getByText(
      "Past the instance's Most protein, 2.5: lower it to 2.5 at most, or no change of your limits is taken.",
    ),
  ).toBeTruthy()
  // Lowering a floor loosens it, which the user confirms.
  type(limit('Least protein'), '2.5')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Loosen' }))
  await screen.findByText('Saved')
  expect(sent['PUT /api/v1/limits']).toEqual([
    { limits: { protein_floor: 2.5, step_kcal: 80 }, confirm: true },
  ])
})

test('a limit outside what any level may have says so, and is not sent', async () => {
  const { sent } = planServer()
  renderAt('/plan/limits')
  await screen.findByRole('form', { name: 'Limits of the plan' })
  fireEvent.click(group('Autopilot'))
  type(limit('Logged days a step needs'), '8')
  expect(screen.getByText('From 0 to 7.')).toBeTruthy()
  fireEvent.click(group('Food'))
  type(limit('Least fat'), '120')
  expect(screen.getByText('From 0 to 100.')).toBeTruthy()
  save('Limits of the plan')
  expect(document.activeElement).toBe(limit('Least fat'))
  expect(sent['PUT /api/v1/limits']).toBeUndefined()
})

test('nothing typed differently says there is nothing to save', async () => {
  const { sent } = planServer()
  renderAt('/plan/limits')
  await screen.findByRole('form', { name: 'Limits of the plan' })
  save('Limits of the plan')
  expect((await screen.findByRole('alert')).textContent).toBe('Nothing to save.')
  expect(sent['PUT /api/v1/limits']).toBeUndefined()
})

test('the question to loosen takes the focus, and a change of a field closes it', async () => {
  planServer()
  renderAt('/plan/limits')
  type(await screen.findByRole('textbox', { name: /^Fastest gain in a bulk/ }), '0.4')
  save('Limits of the plan')
  const back = await screen.findByRole('button', { name: 'Back' })
  expect(document.activeElement).toBe(back)
  expect(document.getElementById(back.getAttribute('aria-describedby') ?? '')?.textContent).toBe(
    'Loosening Fastest gain in a bulk allows a plan the limit refused. Loosen it?',
  )
  type(limit('Fastest gain in a bulk'), '0.45')
  expect(screen.queryByRole('button', { name: 'Back' })).toBeNull()
})

test('a loosening on its way cannot be taken back', async () => {
  planServer({}, { 'PUT /api/v1/limits': () => new Promise<Response>(() => undefined) })
  renderAt('/plan/limits')
  type(await screen.findByRole('textbox', { name: /^Fastest gain in a bulk/ }), '0.4')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Loosen' }))
  await waitFor(() => {
    expect(screen.getByRole('button', { name: 'Back' }).matches(':disabled')).toBe(true)
  })
})

test('a loosening confirmed and refused says why at its field, the question answered', async () => {
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
  renderAt('/plan/limits')
  const bulk = await screen.findByRole('textbox', { name: /^Fastest gain in a bulk/ })
  type(bulk, '0.4')
  save('Limits of the plan')
  fireEvent.click(await screen.findByRole('button', { name: 'Loosen' }))
  const said = await screen.findByText(
    'The server did not take it: "Fastest gain in a bulk" 0.4: want less',
  )
  expect(screen.queryByRole('button', { name: 'Loosen' })).toBeNull()
  expect(bulk.getAttribute('aria-invalid')).toBe('true')
  expect(bulk.getAttribute('aria-describedby')).toContain(said.id)
  expect(document.activeElement).toBe(bulk)
})

test('back from a loosening the server found leaves no word of it', async () => {
  const { setLimits } = planServer()
  renderAt('/plan/limits')
  const bulk = await screen.findByRole('textbox', { name: /^Fastest gain in a bulk/ })
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
  renderAt('/plan/limits')
  const cut = await screen.findByRole('textbox', { name: new RegExp(`^${cutLabel}`) })
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

test("the server's refusal names the limits as the page does, at the field", async () => {
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
  renderAt('/plan/limits')
  await screen.findByRole('form', { name: 'Limits of the plan' })
  fireEvent.click(group('Autopilot'))
  const drift = limit('Most drift from the target you agreed to')
  type(drift, '12')
  // Closed again, the refusal opens it.
  fireEvent.click(group('Autopilot'))
  save('Limits of the plan')
  expect(
    await screen.findByText(
      'The server did not take it: "Most drift from the target you agreed to" 120: want 0 to 100 %',
    ),
  ).toBeTruthy()
  expect(group('Autopilot').getAttribute('aria-expanded')).toBe('true')
  expect(document.activeElement).toBe(drift)
})

test('a refusal that names no limit says why under the form', async () => {
  planServer(
    {},
    {
      'PUT /api/v1/limits': () =>
        Response.json(
          { status: 422, title: 'Unprocessable Entity', detail: 'no such limit: speed' },
          { status: 422, headers: { 'Content-Type': 'application/problem+json' } },
        ),
    },
  )
  renderAt('/plan/limits')
  type(await screen.findByRole('textbox', { name: new RegExp(`^${cutLabel}`) }), '0.6')
  save('Limits of the plan')
  expect((await screen.findByRole('alert')).textContent).toBe(
    'The server did not take it: no such limit: speed',
  )
})

test('in Croatian a limit is typed with a decimal comma', async () => {
  const { sent } = planServer({}, signedIn('hr'))
  renderAt('/plan/limits')
  const cut = await screen.findByRole('textbox', { name: /^Najbrži gubitak u rezu, BMI ispod 30/ })
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
