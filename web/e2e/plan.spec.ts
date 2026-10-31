import { expect, test, type Page } from '@playwright/test'

import { signIn } from './user.ts'

// toPlanRow goes to the plan, as a phone does, and opens a row of its
// list.
async function toPlanRow(page: Page, row: string) {
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Plan' })
    .click()
  // Its own row's: a first step may name it too ("Complete your profile").
  await page.getByRole('link', { name: row, exact: true }).click()
  await expect(page.getByRole('heading', { name: row, level: 1 })).toBeVisible()
}

test('the profile is set, and the plan says it whole', async ({ page }) => {
  await signIn(page)
  await toPlanRow(page, 'Profile')

  const profile = page.getByRole('form', { name: 'Profile' })
  await profile.getByText('Female', { exact: true }).click()
  await profile.getByLabel('Birth date').fill('1990-06-15')
  await profile.getByRole('textbox', { name: /Height/ }).fill('168')
  await profile.getByRole('button', { name: 'Save' }).click()
  await expect(profile.getByText('A plan has all it needs of you.')).toBeVisible()
  await page.getByRole('main').getByRole('link', { name: 'Plan' }).click()
  await expect(page.getByText(/^Female · \d+ years · 168 cm$/)).toBeVisible()

  // The profile cleared again, for the other tests.
  await page.getByRole('link', { name: 'Profile', exact: true }).click()
  await profile.getByText('Not given', { exact: true }).click()
  await profile.getByLabel('Birth date').fill('')
  await profile.getByRole('textbox', { name: /Height/ }).fill('')
  await profile.getByRole('button', { name: 'Save' }).click()
  await expect(profile.getByText('A plan still needs: sex, birth date, height.')).toBeVisible()
})

test('a limit is tightened and loosened again', async ({ page }) => {
  await signIn(page)
  await toPlanRow(page, 'Limits of the plan')

  const limits = page.getByRole('form', { name: 'Limits of the plan' })
  const cut = limits.getByRole('textbox', { name: /^Fastest loss in a cut, BMI under 30/ })
  await cut.fill('0.5')
  await limits.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(cut).toHaveValue('0.5')

  // Back to the default, a loosening, which the user confirms.
  await cut.fill('0.7')
  await limits.getByRole('button', { name: 'Save' }).click()
  await limits.getByRole('button', { name: 'Loosen' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(cut).toHaveValue('0.7')
})

// QA #215: a pace past the instance's, saved, said so only at the foot of
// the form, many fields below the one that was wrong.
test('a wrong limit takes the focus, in view, and says why at its field', async ({ page }) => {
  await signIn(page)
  await toPlanRow(page, 'Limits of the plan')

  const limits = page.getByRole('form', { name: 'Limits of the plan' })
  const cut = limits.getByRole('textbox', { name: /^Fastest loss in a cut, BMI under 30/ })
  await cut.fill('2')
  // A field of another group, far below, then Save.
  await limits.getByRole('button', { name: /^Autopilot/ }).click()
  await limits.getByRole('textbox', { name: /^Weigh-ins a step needs/ }).fill('4')
  await limits.getByRole('button', { name: 'Save' }).click()
  await expect(cut).toBeFocused()
  await expect(cut).toBeInViewport()
  await expect(cut).toHaveAttribute('aria-invalid', 'true')
  await expect(limits.getByText("Past the instance's limit, 1.")).toBeInViewport()
  await expect(page.getByText('Saved', { exact: true })).toBeHidden()

  // Fixed, it is saved; then as it was, for the other tests.
  await cut.fill('0.6')
  await limits.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  expect(
    (
      await page.request.put('/api/v1/limits', {
        data: { limits: { cut_rate: -0.7, weigh_ins: 3 }, confirm: true },
      })
    ).ok(),
  ).toBe(true)
})

// cutFromToday gives the signed-in user what a plan needs (a profile and
// a weigh-in) and a cut from today, as the app makes one; it returns
// today.
async function cutFromToday(page: Page): Promise<string> {
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
  const me: unknown = await (await api.get('/api/v1/me')).json()
  const today = typeof me === 'object' && me !== null && 'today' in me ? String(me.today) : ''
  const made = await api.post('/api/v1/plan/changes', {
    data: { phases: [{ start: today, type: 'cut', rate_pct_week: -0.5 }] },
  })
  expect(made.ok()).toBe(true)
  return today
}

test('the plan is changed in the app, each change a revision taken back', async ({ page }) => {
  await signIn(page)
  const today = await cutFromToday(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Plan' })
    .click()

  // A first target, by type of day.
  const targets = page.getByRole('region', { name: 'Targets by type of day' })
  const sheet = page.getByRole('dialog', { name: 'Targets by type of day', exact: true })
  await targets.getByRole('button', { name: 'Edit targets' }).click()
  const training = sheet.getByRole('group', { name: 'Training' })
  await training.getByRole('textbox', { name: 'kcal' }).fill('2300')
  await training.getByRole('textbox', { name: 'Protein, g' }).fill('160')
  await training.getByRole('textbox', { name: 'Fat, g' }).fill('65')
  const rest = sheet.getByRole('group', { name: 'Rest' })
  await rest.getByRole('textbox', { name: 'kcal' }).fill('2000')
  await rest.getByRole('textbox', { name: 'Protein, g' }).fill('160')
  await rest.getByRole('textbox', { name: 'Fat, g' }).fill('65')
  await expect(rest.getByText('Carbs follow: 194 g')).toBeVisible()
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(sheet).toBeHidden()
  await expect(targets.getByText('2,000 kcal', { exact: true })).toBeVisible()

  // Past the user's floor of kcal, the field says so; within it, saved.
  await targets.getByRole('button', { name: 'Edit targets' }).click()
  await rest.getByRole('textbox', { name: 'kcal' }).fill('1300')
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(rest.getByText('At least 1500 kcal: your limit.')).toBeVisible()
  await expect(sheet.getByRole('link', { name: 'Loosen it in your limits' })).toBeVisible()
  await rest.getByRole('textbox', { name: 'kcal' }).fill('1900')
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(sheet).toBeHidden()
  await expect(targets.getByText('1,900 kcal', { exact: true })).toBeVisible()

  // The phase, faster.
  const hero = page.getByRole('region', { name: 'Cut' })
  await hero.getByRole('button', { name: 'Edit phase' }).click()
  const phase = page.getByRole('dialog', { name: 'Edit phase', exact: true })
  await phase.getByRole('textbox', { name: 'Loss (% a week)' }).fill('0.6')
  await phase.getByRole('button', { name: 'Save' }).click()
  await expect(phase).toBeHidden()
  await expect(hero.getByText('-0.6% a week', { exact: true })).toBeVisible()

  // The history says each, the latest first, and takes it back.
  const history = page.getByRole('region', { name: 'Plan history' })
  const latest = history.getByRole('listitem').first()
  await expect(latest.getByText('Changed in the app.')).toBeVisible()
  await expect(latest.getByText(/, by you$/)).toBeVisible()
  await expect(latest.getByText('-0.5% → -0.6% a week')).toBeVisible()
  await expect(history.getByText('Rest days: 2,000 → 1,900 kcal · C 194 → 169 g')).toBeVisible()
  await latest.getByRole('button', { name: 'Take back' }).click()
  await latest.getByRole('button', { name: 'Take back' }).click()
  await expect(
    history.getByRole('listitem').first().getByText('Taken back in the app.'),
  ).toBeVisible()
  await expect(hero.getByText('-0.5% a week', { exact: true })).toBeVisible()

  // No plan again, and nothing it needed, for the other tests.
  const cleared = await page.request.post('/api/v1/plan/changes', {
    data: { phases: [{ start: today, delete: true }], targets: [{ from: today, delete: true }] },
  })
  expect(cleared.ok()).toBe(true)
  expect((await page.request.put('/api/v1/profile', { data: {} })).ok()).toBe(true)
  expect((await page.request.delete('/api/v1/days/today/metrics/weight')).ok()).toBe(true)
})

test('the next phase is planned in the app', async ({ page }) => {
  await signIn(page)
  const today = await cutFromToday(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Plan' })
    .click()

  const hero = page.getByRole('region', { name: 'Cut' })
  await hero.getByRole('button', { name: 'Next phase' }).click()
  const sheet = page.getByRole('dialog', { name: 'Next phase', exact: true })
  await sheet.getByRole('combobox', { name: 'Type' }).selectOption('maintain')
  await sheet.getByRole('button', { name: 'Save' }).click()
  await expect(sheet).toBeHidden()
  // On the timeline, after the cut.
  await expect(
    hero.getByRole('list', { name: 'Phases' }).getByRole('listitem').filter({ hasText: 'Planned' }),
  ).toContainText('Maintenance')

  // No plan again, and nothing it needed, for the other tests.
  const tomorrow = new Date(`${today}T00:00:00Z`)
  tomorrow.setUTCDate(tomorrow.getUTCDate() + 1)
  const cleared = await page.request.post('/api/v1/plan/changes', {
    data: {
      phases: [
        { start: today, delete: true },
        { start: tomorrow.toISOString().slice(0, 10), delete: true },
      ],
    },
  })
  expect(cleared.ok()).toBe(true)
  expect((await page.request.put('/api/v1/profile', { data: {} })).ok()).toBe(true)
  expect((await page.request.delete('/api/v1/days/today/metrics/weight')).ok()).toBe(true)
})

test('the autopilot is turned on after its question, its day changed, and off again', async ({
  page,
}) => {
  await signIn(page)
  try {
    await toPlanRow(page, 'Autopilot')

    const mode = page.getByRole('group', { name: 'When the review finds a step' })
    await mode.getByText('Autopilot', { exact: true }).click()
    const ask = page.getByRole('region', { name: 'Turn on the autopilot?' })
    await expect(ask.getByText(/^It will change your kcal target by itself/)).toBeVisible()
    await ask.getByRole('button', { name: 'Turn on' }).click()
    await expect(page.getByText('Autopilot on', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Drift' })).toBeVisible()
    await page.reload()
    await expect(mode.getByRole('radio', { name: 'Autopilot' })).toBeChecked()

    const day = page.getByRole('region', { name: 'Review day' })
    await day.getByText('Sat', { exact: true }).click()
    await expect(page.getByText('Review day changed', { exact: true })).toBeVisible()
    await expect(day.getByText(/Next review: Sat, /)).toBeVisible()

    // Back to Mondays, and off, which asks nothing.
    await day.getByText('Mon', { exact: true }).click()
    await expect(day.getByText(/Next review: Mon, /)).toBeVisible()
    await mode.getByText('Ask me first', { exact: true }).click()
    await expect(page.getByText('The autopilot asks first', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Drift' })).toBeHidden()
    await page.reload()
    await expect(mode.getByRole('radio', { name: 'Ask me first' })).toBeChecked()
  } finally {
    // As it was, for the other tests, whatever failed above.
    expect(
      (
        await page.request.put('/api/v1/autopilot', {
          data: { mode: 'propose', weekday: 'monday' },
        })
      ).ok(),
    ).toBe(true)
  }
})
