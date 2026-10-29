import { AxeBuilder } from '@axe-core/playwright'
import { expect, test } from '@playwright/test'

import { listTools } from './mcp.ts'
import { newUser, signIn } from './user.ts'

// The first steps of a user who has given nothing yet: the e2e server's
// second user, whom no other test signs in as.
test("a new user's first steps lead to a plan, one ticked at a time", async ({ page }) => {
  await signIn(page, newUser)
  const steps = page.getByRole('region', { name: 'First steps' })
  await expect(steps).toBeVisible()
  await expect(steps.getByText('0 of 4')).toBeVisible()
  // Nothing measured nor said yet: no empty check-in, no empty tiles.
  await expect(page.getByRole('region', { name: 'Check-in' })).toContainText('How was today?')
  const body = page.getByRole('region', { name: 'Body today' })
  await expect(body.getByRole('button', { name: 'Add a weigh-in' })).toBeVisible()
  await expect(body.getByText('—')).toHaveCount(0)
  const found = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'])
    .analyze()
  expect(found.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical')).toEqual(
    [],
  )

  // The profile, on its page.
  await steps.getByRole('link', { name: 'Complete your profile' }).click()
  const profile = page.getByRole('form', { name: 'Profile' })
  await profile.getByText('Female', { exact: true }).click()
  await profile.getByLabel('Birth date').fill('1992-03-08')
  await profile.getByRole('textbox', { name: /Height/ }).fill('166')
  await profile.getByRole('button', { name: 'Save' }).click()
  await expect(profile.getByText('A plan has all it needs of you.')).toBeVisible()
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Today' })
    .click()
  await expect(steps.getByText('1 of 4')).toBeVisible()

  // The first weigh-in, in the card, Enter to save.
  await steps.getByLabel('First weigh-in').fill('64.5')
  await steps.getByLabel('First weigh-in').press('Enter')
  await expect(page.getByText('Saved: Weight')).toBeVisible()
  await expect(steps.getByText('2 of 4')).toBeVisible()
  await expect(body.getByRole('button', { name: /^Weight 64\.5 kg/ })).toBeVisible()

  // The first meal: + opens the meal to log; this one an AI coach logs.
  await steps.getByRole('link', { name: 'Log your first meal' }).click()
  const meal = page.getByRole('dialog', { name: 'New meal' })
  await expect(meal).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(meal).toBeHidden()
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Porridge',
      items: [{ name: 'Porridge', nutrients: { kcal: 320, protein: 12, carbs: 52, fat: 7 } }],
    },
  })
  expect(logged.ok()).toBe(true)
  await page.reload()
  await expect(steps.getByText('3 of 4')).toBeVisible()

  // What is left: the AI coach, connected from the choice.
  await steps.getByRole('link', { name: 'Connect your AI coach or a phone' }).click()
  const sheet = page.getByRole('dialog', { name: 'Connect' })
  await expect(sheet.getByRole('heading', { name: 'What do you connect?' })).toBeVisible()
  await sheet.getByRole('button', { name: /^AI coach/ }).click()
  await sheet.getByRole('button', { name: 'Create the access key' }).click()
  const key =
    (await sheet
      .getByRole('group', { name: 'The access key, shown only now', exact: true })
      .locator('code')
      .textContent()) ?? ''
  await listTools(page, key)
  await expect(sheet.getByText('Connected', { exact: true })).toBeVisible()
  await sheet.getByRole('button', { name: 'Done' }).click()

  // All four: what to ask the AI coach for.
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Today' })
    .click()
  await expect(steps.getByText('4 of 4')).toBeVisible()
  await expect(steps.getByText('Ask your AI coach for a plan')).toBeVisible()
  await expect(
    steps.getByRole('group', { name: 'Write this to your AI coach' }).locator('code'),
  ).toHaveText('Look at my data and propose a plan.')

  // Put off for later, and back from the plan.
  await steps.getByRole('button', { name: 'Later' }).click()
  await expect(steps).toBeHidden()
  await expect(page.getByText('A plan gives the day its target.')).toBeVisible()
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Plan' })
    .click()
  await page.getByRole('link', { name: 'Show the first steps' }).click()
  await expect(steps.getByText('4 of 4')).toBeVisible()
})
