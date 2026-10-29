import { expect, test } from '@playwright/test'

import { openMeal, signIn, toSettings } from './user.ts'

test('a meal deleted comes back from the recent changes, after its toast is gone', async ({
  page,
}) => {
  await signIn(page)
  const name = `Quinoa salad ${String(Date.now())}`
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: name,
      items: [{ name, nutrients: { kcal: 460, protein: 16, carbs: 60, fat: 15 } }],
    },
  })
  expect(logged.ok()).toBe(true)
  await page.goto('/')
  const meal = page.getByRole('article', { name })
  await openMeal(meal)
  await meal.getByRole('button', { name: `Delete: ${name}` }).click()
  await expect(meal).toHaveCount(0)

  // The toast goes: closed by its button.
  const toasts = page.getByRole('region', { name: 'Notifications' })
  await expect(toasts).toContainText(`Deleted: ${name}`)
  await toasts.getByLabel('Close').click()
  await expect(toasts.getByRole('button', { name: 'Undo' })).toHaveCount(0)

  // The recent changes, from the day's header: the delete, by you, and
  // its Undo, the latest's alone.
  await page.getByRole('button', { name: 'Recent changes' }).click()
  const sheet = page.getByRole('dialog', { name: 'Recent changes' })
  const rows = sheet.getByRole('listitem')
  await expect(rows.first()).toContainText(`Deleted: ${name}`)
  await expect(rows.first()).toContainText('· you')
  await expect(rows.nth(1)).toContainText(`Logged: ${name}`)
  await expect(sheet.getByRole('button', { name: /^Undo/ })).toHaveCount(1)
  await sheet.getByRole('button', { name: `Undo: Deleted: ${name}` }).click()

  await expect(toasts).toContainText('Taken back: a meal deleted.')
  await expect(rows.first()).toContainText('Taken back')
  // The list follows: the meal's log is the next to take back.
  await expect(sheet.getByRole('button', { name: `Undo: Logged: ${name}` })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(meal).toHaveCount(1)
})

test('the settings lead to the recent changes too', async ({ page }) => {
  await signIn(page)
  await toSettings(page)
  await page.getByRole('button', { name: 'Recent changes' }).click()
  const sheet = page.getByRole('dialog', { name: 'Recent changes' })
  await expect(sheet.getByRole('listitem').first()).toBeVisible()
})
