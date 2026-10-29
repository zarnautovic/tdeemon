import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('a meal eaten at this hour before is suggested above the search, and logged again in two taps', async ({
  page,
}) => {
  await signIn(page)
  // Meals logged as a model would, through the API: one two days before,
  // now (the day before is the day's test's), and one just now.
  const logged = await Promise.all(
    [
      { name: 'Chickpea curry', at: Date.now() - 2 * 86_400_000 },
      { name: 'Lentil soup', at: Date.now() },
    ].map(async ({ name, at }) => {
      const meal = await page.request.post('/api/v1/meals', {
        data: {
          description: name,
          eaten_at: new Date(at).toISOString(),
          items: [{ name, nutrients: { kcal: 540, protein: 22, carbs: 70, fat: 18 } }],
        },
      })
      return meal.ok()
    }),
  )
  expect(logged).toEqual([true, true])
  await page.goto('/')
  const curries = page.getByRole('article').filter({ hasText: 'Chickpea curry' })
  await expect(page.getByRole('article').filter({ hasText: 'Lentil soup' })).toHaveCount(1)
  await expect(curries).toHaveCount(0)

  // One: the bar's +. Two: the card's Repeat, above the search; the meal
  // just logged is no card.
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Log a meal' })
    .click()
  const sheet = page.getByRole('dialog', { name: 'A new meal' })
  const cards = sheet.getByRole('region', { name: /^For / })
  await expect(cards.getByRole('button', { name: 'Repeat: Lentil soup' })).toHaveCount(0)
  await cards.getByRole('button', { name: 'Repeat: Chickpea curry' }).click()

  await expect(page.getByText('Repeated: Chickpea curry · 540 kcal')).toBeVisible()
  await expect(sheet).toBeHidden()
  await expect(curries).toHaveCount(1)
})

test('on a phone the keyboard waits for a tap on the search', async ({ page }) => {
  await signIn(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Log a meal' })
    .click()
  const sheet = page.getByRole('dialog', { name: 'A new meal' })
  const search = sheet.getByLabel('Find a food or a meal')
  await expect(sheet).toBeFocused()
  await expect(search).not.toBeFocused()

  await search.tap()
  await expect(search).toBeFocused()
})

test.describe('on a desktop', () => {
  test.use({ viewport: { width: 1440, height: 900 }, isMobile: false, hasTouch: false })

  test('the search has the focus, opened by its key', async ({ page }) => {
    await signIn(page)
    await page.keyboard.press('n')
    const sheet = page.getByRole('dialog', { name: 'A new meal' })
    await expect(sheet.getByLabel('Find a food or a meal')).toBeFocused()
  })
})

test('a food not found is made in a step of its own, and goes into the meal logged', async ({
  page,
}) => {
  await signIn(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Log a meal' })
    .click()
  const sheet = page.getByRole('dialog', { name: 'A new meal' })
  await sheet.getByLabel('Find a food or a meal').fill('kefir')
  await expect(
    sheet.getByText('Nothing of yours matches: give an estimate, or make a new food.'),
  ).toBeVisible()
  await sheet.getByRole('button', { name: 'A new food' }).click()

  // In the search's place, with one button: the meal's is not there.
  await expect(sheet.getByRole('button', { name: 'Back to search' })).toBeVisible()
  await expect(sheet.getByRole('button', { name: 'Log the meal' })).toBeHidden()
  await expect(sheet.getByLabel('Name', { exact: true })).toHaveValue('kefir')
  await sheet.getByLabel('Name', { exact: true }).fill('Kefir')
  await sheet.getByText('per 100 ml').click()
  await sheet.getByLabel('Energy (kcal)').fill('41')
  await sheet.getByLabel('Protein (g)').fill('3.4')
  await sheet.getByLabel('Carbs (g)').fill('4')
  await sheet.getByLabel('Fat (g)', { exact: true }).fill('1.5')
  await sheet.getByLabel('Eaten now (ml)').fill('250')
  await sheet.getByRole('button', { name: 'Add to meal' }).click()

  await expect(sheet.getByRole('list', { name: 'In the meal' })).toContainText(
    'Kefir · 250 ml · 103 kcal',
  )
  await expect(sheet.getByLabel('Find a food or a meal')).toHaveValue('')
  await sheet.getByRole('button', { name: 'Log 1 item · 103 kcal' }).click()
  await expect(page.getByText(/^Logged: Kefir · 103 kcal/)).toBeVisible()
  await expect(sheet).toBeHidden()
  await expect(page.getByRole('article').filter({ hasText: 'Kefir' })).toHaveCount(1)
})

test("the day's chips scroll on their own, not the page", async ({ page }) => {
  await signIn(page)
  // Foods of long names, each eaten: chips wider than a phone together.
  const names = ['Greek yoghurt with honey', 'Wholegrain sourdough', 'Smoked salmon slices']
  const logged = await Promise.all(
    names.map(async (name) => {
      const food = await page.request.post('/api/v1/foods', {
        data: {
          name,
          basis: '100g',
          source: 'label',
          nutrients: { kcal: 200, protein: 10, carbs: 20, fat: 8 },
        },
      })
      const saved: unknown = await food.json()
      const id =
        typeof saved === 'object' && saved !== null && 'id' in saved && typeof saved.id === 'string'
          ? saved.id
          : ''
      const meal = await page.request.post('/api/v1/meals', {
        data: { items: [{ food_id: id, amount: 150 }] },
      })
      return meal.ok()
    }),
  )
  expect(logged).toEqual([true, true, true])
  await page.goto('/')
  await expect(page.getByRole('region', { name: 'Quick add' })).toBeVisible()

  // A string: the tests' types know no DOM.
  const width = await page.evaluate<number>('document.documentElement.scrollWidth')
  expect(width).toBe(page.viewportSize()?.width)
})
