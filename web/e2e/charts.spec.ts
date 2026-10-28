import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('the progress says how the phase goes, and shows the days logged and their weeks', async ({
  page,
}) => {
  await signIn(page)
  // A weigh-in of today and a meal of three days ago, as a phone and a
  // model would send them: today's day is open, and counts in no average
  // yet (and the day before is the day's test's).
  expect(
    (await page.request.put('/api/v1/days/today/metrics/weight', { data: { value: 80.5 } })).ok(),
  ).toBe(true)
  expect(
    (
      await page.request.post('/api/v1/meals', {
        data: {
          eaten_at: new Date(Date.now() - 3 * 86_400_000).toISOString(),
          items: [{ name: 'Soup', nutrients: { kcal: 400, protein: 20, carbs: 40, fat: 15 } }],
        },
      })
    ).ok(),
  ).toBe(true)

  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Progress' })
    .click()

  // The user has no plan: the hero leads to one.
  const verdict = page.getByRole('region', { name: 'The phase so far' })
  await expect(verdict).toContainText('No phase yet.')
  await expect(verdict.getByRole('link', { name: 'Set up a plan' })).toBeVisible()

  const weight = page.getByRole('region', { name: 'Weight' })
  await expect(
    weight.getByRole('img', { name: 'Trend weight from 80.5 to 80.5 kg, 1 weigh-in.' }),
  ).toBeVisible()
  await expect(weight.locator('svg').first()).toBeVisible()
  await page.getByText('30 d', { exact: true }).click()
  await expect(page).toHaveURL(/days=30/)
  await expect(page.getByRole('radio', { name: '30 days' })).toBeChecked()
  await expect(page.getByRole('region', { name: 'TDEE and intake' })).toContainText(
    'Eaten 400 kcal a day, over 1 logged day',
  )
  await expect(page.getByRole('table', { name: 'By week' })).toContainText('400')
})
