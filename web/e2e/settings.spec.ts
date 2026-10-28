import { expect, test } from '@playwright/test'

import { signIn, toSettings } from './user.ts'

test("the account's name and time zone are saved, and come back", async ({ page }) => {
  await signIn(page)
  await toSettings(page)
  await page.getByRole('link', { name: 'Account, language and day' }).click()

  const you = page.getByRole('form', { name: 'Account, language and day' })
  await you.getByLabel('Name', { exact: true }).fill('Ana M.')
  // The time zone is found by a part of its name, an old one too.
  const zone = you.getByRole('combobox', { name: 'Time zone' })
  await expect(zone).toHaveValue('Europe/Zagreb')
  await zone.fill('kiev')
  await page.getByRole('option', { name: 'Europe/Kyiv' }).click()
  await you.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(you.getByLabel('Name', { exact: true })).toHaveValue('Ana M.')
  await expect(zone).toHaveValue('Europe/Kyiv')

  // The list says them.
  await page.getByRole('main').getByRole('link', { name: 'Plan' }).click()
  await expect(page.getByText('Signed in as Ana M.')).toBeVisible()
  await expect(page.getByText('English · Europe/Kyiv · the day from 04:00')).toBeVisible()

  // As they were, for the other tests.
  await page.getByRole('link', { name: 'Account, language and day' }).click()
  await you.getByLabel('Name', { exact: true }).fill('Ana')
  await zone.fill('zagreb')
  await page.getByRole('option', { name: 'Europe/Zagreb' }).click()
  await you.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(zone).toHaveValue('Europe/Zagreb')
})

test('the notes for a coach are saved, and come back', async ({ page }) => {
  await signIn(page)
  await toSettings(page)
  await page.getByRole('link', { name: 'Notes for a coach' }).click()

  const notes = page.getByRole('textbox', { name: 'Notes for a coach' })
  await notes.fill('Knees: no running.')
  await page
    .getByRole('form', { name: 'Notes for a coach' })
    .getByRole('button', { name: 'Save' })
    .click()
  await expect(page.getByText('The notes for a coach are saved')).toBeVisible()
  await page.reload()
  await expect(notes).toHaveValue('Knees: no running.')

  // Cleared, for the other tests.
  await notes.fill('')
  await page
    .getByRole('form', { name: 'Notes for a coach' })
    .getByRole('button', { name: 'Save' })
    .click()
  await expect(page.getByText('The notes for a coach are saved')).toBeVisible()
})

test('a new token is shown once and copied, and works until all are revoked', async ({ page }) => {
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await signIn(page)
  await toSettings(page)
  await page.getByRole('link', { name: 'Devices and AI clients' }).click()

  await page.getByRole('button', { name: 'New token' }).click()
  const sheet = page.getByRole('dialog', { name: 'New token' })
  const name = sheet.getByLabel('Name')
  await expect(name).toBeFocused()
  await expect(name).toHaveAttribute('placeholder', 'My phone')
  await name.fill('Test phone')
  await sheet.getByRole('button', { name: 'Create the token' }).click()
  const token = (await sheet.locator('code').textContent()) ?? ''
  expect(token).toMatch(/^tdm_/)
  await sheet.getByRole('button', { name: 'Copy' }).click()
  await expect(sheet.getByRole('button', { name: 'Copied' })).toBeVisible()
  // The e2e tests' types have no DOM: the page reads its clipboard.
  expect(await page.evaluate<string>('navigator.clipboard.readText()')).toBe(token)
  // Shown once: closed, it is gone, and the list has its card.
  await sheet.getByRole('button', { name: 'Done' }).click()
  await expect(sheet).toBeHidden()
  await expect(page.getByText(token)).toBeHidden()
  const tokens = page.getByRole('region', { name: 'Tokens' })
  await expect(tokens.getByRole('heading', { name: 'Test phone' })).toBeVisible()
  await expect(tokens.getByText('Send health data')).toBeVisible()

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
  await page.getByRole('link', { name: 'Appearance' }).click()
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
