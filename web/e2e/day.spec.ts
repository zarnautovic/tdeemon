import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('the day shows what was logged, and the days before it', async ({ page }) => {
  await signIn(page)
  // Logged as a model would, through the API, with the page's session.
  const meal = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Lunch',
      items: [
        { name: 'Chicken with rice', nutrients: { kcal: 640, protein: 45, carbs: 70, fat: 15 } },
      ],
    },
  })
  expect(meal.ok()).toBe(true)
  const weighing = await page.request.put('/api/v1/days/today/metrics/weight', {
    data: { value: 72.5 },
  })
  expect(weighing.ok()).toBe(true)

  await page.goto('/')

  const lunch = page.getByRole('article').filter({ hasText: 'Lunch' })
  await expect(lunch).toContainText('640 kcal')
  await expect(lunch).toContainText('Chicken with rice')
  await expect(page.getByRole('region', { name: 'Measurements' })).toContainText('72.5 kg')

  await page.getByRole('link', { name: 'The day before' }).click()
  await expect(page.getByText('Nothing logged yet.')).toBeVisible()
  await page.getByRole('link', { name: 'Back to today' }).click()
  await expect(page.getByRole('heading', { name: 'Today' })).toBeVisible()
})

test("a meal's energy stays on one line beside a long description", async ({ page }) => {
  await signIn(page)
  const long = 'Grilled chicken with roasted vegetables, rice and a yoghurt sauce'
  const logged = await Promise.all(
    (
      [
        ['Espresso', 20],
        [long, 1180],
      ] as const
    ).map(([description, kcal]) =>
      page.request.post('/api/v1/meals', {
        data: {
          description,
          items: [{ name: 'The plate', nutrients: { kcal, protein: 1, carbs: 1, fat: 1 } }],
        },
      }),
    ),
  )
  expect(logged.every((r) => r.ok())).toBe(true)

  await page.goto('/')
  const energy = (description: string, kcal: string) =>
    page.getByRole('article').filter({ hasText: description }).getByText(kcal, { exact: true })
  // As high as a short meal's energy, which has room: one line.
  const short = await energy('Espresso', '20 kcal').boundingBox()
  const beside = await energy(long, '1,180 kcal').boundingBox()
  expect(short).not.toBeNull()
  expect(beside?.height).toBe(short?.height)
})
