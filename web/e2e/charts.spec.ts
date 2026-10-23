import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('the charts show the days logged, as charts and in words', async ({ page }) => {
  await signIn(page)
  // A weigh-in and a meal of today, as a phone and a model would send them.
  expect(
    (await page.request.put('/api/v1/days/today/metrics/weight', { data: { value: 80.5 } })).ok(),
  ).toBe(true)
  expect(
    (
      await page.request.post('/api/v1/meals', {
        data: {
          items: [{ name: 'Soup', nutrients: { kcal: 400, protein: 20, carbs: 40, fat: 15 } }],
        },
      })
    ).ok(),
  ).toBe(true)

  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Charts' })
    .click()

  const weight = page.getByRole('region', { name: 'Weight and trend' })
  await expect(weight).toContainText('Trend weight from 80.5 to 80.5 kg.')
  await expect(weight.locator('svg').first()).toBeVisible()
  await page.getByRole('button', { name: '30 days' }).click()
  await expect(page).toHaveURL(/days=30/)
  await expect(page.getByRole('region', { name: 'What was eaten' })).toContainText(
    '400 kcal a day on average',
  )
})
