import { expect, test } from '@playwright/test'

import { listTools } from './mcp.ts'
import { signIn } from './user.ts'

test("the plan's Connect an AI coach gives the steps, and sees the first call come", async ({
  page,
  baseURL,
}) => {
  await signIn(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Plan' })
    .click()
  await page.getByRole('link', { name: 'Connect an AI coach' }).click()

  // An AI coach's key, named, with nothing to pick.
  const sheet = page.getByRole('dialog', { name: 'Connect' })
  await expect(sheet.getByLabel('Name')).toHaveValue('Claude')
  await expect(sheet.getByRole('checkbox')).toHaveCount(0)
  await sheet.getByRole('button', { name: 'Create the access key' }).click()

  // The instance's MCP address, the key, and Claude Code's command with
  // both.
  const field = (label: string) =>
    sheet.getByRole('group', { name: label, exact: true }).locator('code')
  await expect(field("The server's address (MCP)")).toHaveText(`${baseURL ?? ''}/mcp`)
  const key = (await field('The access key, shown only now').textContent()) ?? ''
  expect(key).toMatch(/^tdm_/)
  await expect(field('Claude Code: run this in a terminal')).toHaveText(
    `claude mcp add --transport http tdeemon ${baseURL ?? ''}/mcp --header "Authorization: Bearer ${key}"`,
  )
  await expect(sheet.getByText('Waiting for the first call…')).toBeVisible()

  // The AI coach calls; the sheet sees it, without a reload.
  await listTools(page, key)
  await expect(sheet.getByText('Connected', { exact: true })).toBeVisible()
  await expect(sheet.getByText(/^· last call /)).toBeVisible()

  // Its card says so too.
  await sheet.getByRole('button', { name: 'Done' }).click()
  const card = page
    .getByRole('region', { name: 'Connected' })
    .getByRole('listitem')
    .filter({ has: page.getByRole('heading', { name: 'Claude' }) })
    .last()
  await expect(card).toContainText('Last used')
  await expect(card).toContainText('Reads and logs')
})
