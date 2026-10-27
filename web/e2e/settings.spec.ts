import { expect, test } from '@playwright/test'

import { signIn, toSettings } from './user.ts'

test("a user's name and notes are set, and a token made and all revoked", async ({ page }) => {
  await signIn(page)
  await toSettings(page)

  const you = page.getByRole('form', { name: 'You' })
  await you.getByLabel('Name', { exact: true }).fill('Ana M.')
  await you.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Signed in as Ana M.')).toBeVisible()

  await page.getByLabel('Notes for a coach').fill('Knees: no running.')
  await page
    .getByLabel('Notes for a coach')
    .locator('xpath=ancestor::form')
    .getByRole('button', { name: 'Save' })
    .click()
  await expect(page.getByText('The notes for a coach are saved')).toBeVisible()
  await page.reload()
  await expect(page.getByLabel('Notes for a coach')).toHaveValue('Knees: no running.')

  const tokens = page.getByRole('region', { name: 'API tokens' })
  await tokens.getByLabel('Name of the token').fill('Test phone')
  await tokens.getByRole('button', { name: 'Create a token' }).click()
  const token = (await tokens.locator('code').textContent()) ?? ''
  expect(token).toMatch(/^tdm_/)
  // The token works, until all are revoked.
  const who = await page.request.get('/api/v1/auth/whoami', {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(who.ok()).toBe(true)
  await tokens.getByRole('button', { name: 'Revoke all tokens' }).click()
  await tokens.getByRole('button', { name: 'Revoke', exact: true }).click()
  await expect(tokens).toContainText('revoked.')
  const after = await page.request.get('/api/v1/auth/whoami', {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(after.status()).toBe(401)

  // The name back, for the other tests.
  await you.getByLabel('Name', { exact: true }).fill('Ana')
  await you.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Signed in as Ana (')).toBeVisible()
})

test("the theme is the system's until one is chosen, and kept in the browser", async ({ page }) => {
  // Geist comes from the app itself, which the content security policy
  // lets in (its Latin part here; a part loads for the letters shown).
  const font = page.waitForResponse((r) =>
    /\/assets\/geist-latin-wght-normal-.*\.woff2$/.test(r.url()),
  )
  await page.emulateMedia({ colorScheme: 'dark' })
  await signIn(page)
  const body = page.locator('body')
  await expect(body).toHaveCSS('background-color', 'rgb(10, 14, 21)')

  await toSettings(page)
  const theme = page.getByRole('group', { name: 'Appearance' })
  await theme.getByText('Light', { exact: true }).click()
  await expect(body).toHaveCSS('background-color', 'rgb(245, 246, 248)')
  await page.reload()
  await expect(body).toHaveCSS('background-color', 'rgb(245, 246, 248)')
  await expect(theme.getByRole('radio', { name: 'Light' })).toBeChecked()

  await theme.getByText('System', { exact: true }).click()
  await expect(body).toHaveCSS('background-color', 'rgb(10, 14, 21)')
  expect((await font).ok()).toBe(true)
})
