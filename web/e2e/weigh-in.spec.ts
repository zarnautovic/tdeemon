import { expect, test, type Page } from '@playwright/test'

import { signIn } from './user.ts'

// todayOf is the user's day now, as the server says it.
async function todayOf(page: Page): Promise<string> {
  const me: unknown = await (await page.request.get('/api/v1/me')).json()
  return typeof me === 'object' && me !== null && 'today' in me ? String(me.today) : ''
}

// yesterdayOf is the day before a date (YYYY-MM-DD).
function yesterdayOf(date: string): string {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() - 1)
  return d.toISOString().slice(0, 10)
}

// noWeight takes a day's weigh-in away, today's unless another is given,
// as other tests may have left one.
async function noWeight(page: Page, date = 'today') {
  const gone = await page.request.delete(`/api/v1/days/${date}/metrics/weight`)
  expect([200, 404]).toContain(gone.status())
}

// weighedYesterday gives the user a weigh-in by hand of the day before
// today, which today's field hints with, and returns that day.
async function weighedYesterday(page: Page): Promise<string> {
  const yesterday = yesterdayOf(await todayOf(page))
  const given = await page.request.put(`/api/v1/days/${yesterday}/metrics/weight`, {
    data: { value: 85.2 },
  })
  expect(given.ok()).toBe(true)
  return yesterday
}

test("the weight is a tap from today's body, and saved with Enter", async ({ page }) => {
  await signIn(page)
  await noWeight(page)
  const yesterday = await weighedYesterday(page)
  await page.reload()

  const body = page.getByRole('region', { name: 'Body today' })
  await body.getByRole('button', { name: /^Weight/ }).click()
  const field = body.getByLabel('Weight')
  await expect(field).toBeFocused()
  await expect(field).toHaveAttribute('placeholder', '85.2')
  await field.fill('71.8')
  await field.press('Enter')
  const toasts = page.getByRole('region', { name: 'Notifications' })
  await expect(toasts).toContainText('Saved: Weight')
  await expect(toasts.getByRole('button', { name: 'Undo' })).toBeVisible()
  await expect(body.getByRole('button', { name: /^Weight 71\.8 kg/ })).toBeFocused()

  // Another tile opens the measurements at its own field.
  await body.getByRole('button', { name: /^Steps/ }).click()
  const sheet = page.getByRole('dialog', { name: 'Measurements' })
  await expect(sheet.getByLabel('Measurement')).toHaveValue('steps')
  await expect(sheet.getByLabel('Value')).toBeFocused()
  await page.keyboard.press('Escape')

  await noWeight(page)
  await noWeight(page, yesterday)
})

test("in the morning, today's hero asks for the weigh-in, and the line goes once given", async ({
  page,
}) => {
  await signIn(page)
  // A plan, so the day has its hero: what a plan needs (a profile and a
  // weigh-in), a cut and its targets from today; then today without a
  // weigh-in.
  const api = page.request
  expect(
    (
      await api.put('/api/v1/profile', {
        data: { sex: 'male', birth_date: '1990-06-15', height_cm: 180 },
      })
    ).ok(),
  ).toBe(true)
  expect((await api.put('/api/v1/days/today/metrics/weight', { data: { value: 85 } })).ok()).toBe(
    true,
  )
  const today = await todayOf(page)
  const day = { kcal: 2200, protein: 160, fat: 65 }
  const made = await api.post('/api/v1/plan/changes', {
    data: {
      phases: [{ start: today, type: 'cut', rate_pct_week: -0.5 }],
      targets: [{ from: today, training: day, rest: day }],
    },
  })
  expect(made.ok()).toBe(true)
  await noWeight(page)
  const yesterday = await weighedYesterday(page)

  // 08:00 or so on the user's clock (Zagreb's, an hour or two ahead of
  // UTC).
  await page.clock.setFixedTime(new Date(`${today}T06:30:00Z`))
  await page.goto('/')
  const hero = page.getByRole('region', { name: 'Energy and macros' })
  const field = hero.getByLabel('Morning weigh-in?')
  await expect(field).toHaveAttribute('placeholder', '85.2')
  await field.fill('84.6')
  await hero.getByRole('button', { name: 'Save the weigh-in' }).click()
  await expect(page.getByText('Saved: Weight')).toBeVisible()
  await expect(field).toBeHidden()
  await expect(
    page.getByRole('region', { name: 'Body today' }).getByRole('button', { name: /^Weight 84\.6/ }),
  ).toBeVisible()

  // No plan again, and nothing it needed, for the other tests.
  const cleared = await api.post('/api/v1/plan/changes', {
    data: { phases: [{ start: today, delete: true }], targets: [{ from: today, delete: true }] },
  })
  expect(cleared.ok()).toBe(true)
  expect((await api.put('/api/v1/profile', { data: {} })).ok()).toBe(true)
  await noWeight(page)
  await noWeight(page, yesterday)
})
