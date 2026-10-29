import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

// A browser that gives no camera, as these tests' does: the barcode's
// digits are typed. Open Food Facts is off (scripts/e2e-server.sh), so a
// barcode none of the user's foods has is nobody's.
test('a barcode nobody has is a new food of it, and the next scan finds that food', async ({
  page,
}) => {
  await signIn(page)
  const open = async () => {
    await page
      .getByRole('navigation', { name: 'Sections' })
      .getByRole('button', { name: 'Log a meal' })
      .click()
    const sheet = page.getByRole('dialog', { name: 'New meal' })
    await sheet.getByRole('button', { name: 'Scan a barcode' }).click()
    await sheet.getByLabel('Or type the digits under the bars').fill('4006381333931')
    await sheet.getByRole('button', { name: 'Look up the code' }).click()
    return sheet
  }

  let sheet = await open()
  await expect(sheet.getByText(/^Nobody has 4006381333931 yet\./)).toBeVisible()
  await sheet.getByLabel('Name', { exact: true }).fill('Spearmint gum')
  await sheet.getByLabel('Energy (kcal)').fill('160')
  await sheet.getByLabel('Protein (g)').fill('0')
  await sheet.getByLabel('Carbs (g)').fill('64')
  await sheet.getByLabel('Fat (g)', { exact: true }).fill('0')
  await sheet.getByLabel('Eaten now (g)').fill('5')
  await sheet.getByRole('button', { name: 'Add to meal' }).click()
  await sheet.getByRole('button', { name: 'Log 1 item · 8 kcal' }).click()
  await expect(page.getByText(/^Logged: Spearmint gum · 8 kcal/)).toBeVisible()

  sheet = await open()
  await expect(sheet.getByText('Your food · 160 kcal per 100 g')).toBeVisible()
  await expect(sheet.getByLabel('Amount: Spearmint gum')).toHaveValue('5')
})
