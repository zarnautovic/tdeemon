import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

test('a food is made, logged, found, corrected, archived and brought back', async ({ page }) => {
  await signIn(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Foods' })
    .click()

  await page.getByRole('link', { name: 'Add a food' }).first().click()
  await page.getByLabel('Name', { exact: true }).fill('Buckwheat')
  // A slip of the energy, which the macros tell, and their energy taken.
  await page.getByLabel('Energy (kcal)').fill('34')
  await page.getByLabel('Protein (g)').fill('13.3')
  await page.getByLabel('Carbs (g)').fill('71.5')
  await page.getByLabel('Fat (g)', { exact: true }).fill('3.4')
  await expect(page.getByText('The macros make about 370 kcal, not 34')).toBeVisible()
  await page.getByRole('button', { name: 'Use 370 kcal' }).click()
  await expect(page.getByLabel('Energy (kcal)')).toHaveValue('370')
  await page.getByRole('button', { name: 'Add another name' }).click()
  await expect(page.getByLabel('Other name')).toBeFocused()
  await page.getByLabel('Other name').fill('kasha')
  await page.getByRole('button', { name: 'Save the food' }).click()
  await expect(page.getByRole('heading', { name: 'Buckwheat' })).toBeVisible()

  // Logged from its page: the meal to log opens with it.
  await page.getByRole('button', { name: 'Log it' }).click()
  const sheet = page.getByRole('dialog', { name: 'A new meal' })
  await sheet.getByRole('button', { name: 'Log 1 item · 370 kcal' }).click()
  await expect(page.getByText(/^Logged: .* · 370 kcal$/)).toBeVisible()
  await expect(page.getByText(/^1× · last /)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Log 100 g' })).toBeVisible()

  await page.getByRole('link', { name: 'Back to the foods' }).click()
  await page.getByLabel('Search the foods').fill('kash')
  await page
    .getByRole('list', { name: 'Found' })
    .getByRole('link', { name: /Buckwheat/ })
    .click()
  await page.getByRole('link', { name: 'Edit' }).click()
  await page.getByLabel('Energy (kcal)').fill('346')
  await page.getByRole('button', { name: 'Save the food' }).click()
  await expect(page.getByText('Saved: Buckwheat')).toBeVisible()
  await expect(page.getByText('kcal per 100 g')).toBeVisible()

  await page.getByRole('button', { name: 'Archive' }).click()
  await expect(
    page.getByText('Archived: only a search of the archived foods finds it'),
  ).toBeVisible()
  await page.getByRole('link', { name: 'Back to the foods' }).click()
  await expect(page.getByRole('list', { name: 'Found' })).toHaveCount(0)
  await page.getByLabel('Search the foods').fill('')
  await page.getByRole('group', { name: 'Lists' }).getByText('Archive', { exact: true }).click()
  await expect(page.getByRole('radio', { name: 'Archive' })).toBeChecked()
  const archived = page.getByRole('list', { name: /^Archive/ })
  await expect(archived).toContainText('346 kcal')
  // A search of the archive is of the archived foods.
  await page.getByLabel('Search the foods').fill('buck')
  await expect(page.getByRole('list', { name: 'Found' })).toContainText('Buckwheat')
  await page.getByLabel('Search the foods').fill('')
  await archived.getByRole('link', { name: /Buckwheat/ }).click()
  await page.getByRole('button', { name: 'Bring back' }).click()
  await expect(page.getByRole('button', { name: 'Archive' })).toBeVisible()
})
