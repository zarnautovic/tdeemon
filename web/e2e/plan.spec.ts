import { expect, test, type Page } from '@playwright/test'

import { signIn } from './user.ts'

// toPlanRow goes to the plan, as a phone does, and opens a row of its
// list.
async function toPlanRow(page: Page, row: string) {
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Plan' })
    .click()
  // Its own row's: a first step may name it too ("Complete your profile").
  await page.getByRole('link', { name: row, exact: true }).click()
  await expect(page.getByRole('heading', { name: row, level: 1 })).toBeVisible()
}

test('the profile is set, and the plan says it whole', async ({ page }) => {
  await signIn(page)
  await toPlanRow(page, 'Profile')

  const profile = page.getByRole('form', { name: 'Profile' })
  await profile.getByText('Female', { exact: true }).click()
  await profile.getByLabel('Birth date').fill('1990-06-15')
  await profile.getByRole('textbox', { name: /Height/ }).fill('168')
  await profile.getByRole('button', { name: 'Save' }).click()
  await expect(profile.getByText('A plan has all it needs of you.')).toBeVisible()
  await page.getByRole('main').getByRole('link', { name: 'Plan' }).click()
  await expect(page.getByText(/^Female · \d+ years · 168 cm$/)).toBeVisible()

  // The profile cleared again, for the other tests.
  await page.getByRole('link', { name: 'Profile', exact: true }).click()
  await profile.getByText('Not given', { exact: true }).click()
  await profile.getByLabel('Birth date').fill('')
  await profile.getByRole('textbox', { name: /Height/ }).fill('')
  await profile.getByRole('button', { name: 'Save' }).click()
  await expect(profile.getByText('A plan still needs: sex, birth date, height.')).toBeVisible()
})

test('a limit is tightened and loosened again', async ({ page }) => {
  await signIn(page)
  await toPlanRow(page, 'Limits of the plan')

  const limits = page.getByRole('form', { name: 'Limits of the plan' })
  const cut = limits.getByRole('textbox', { name: /^Fastest loss in a cut, BMI under 30/ })
  await cut.fill('0.5')
  await limits.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(cut).toHaveValue('0.5')

  // Back to the default, a loosening, which the user confirms.
  await cut.fill('0.7')
  await limits.getByRole('button', { name: 'Save' }).click()
  await limits.getByRole('button', { name: 'Loosen' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  await page.reload()
  await expect(cut).toHaveValue('0.7')
})

// QA #215: a pace past the instance's, saved, said so only at the foot of
// the form, many fields below the one that was wrong.
test('a wrong limit takes the focus, in view, and says why at its field', async ({ page }) => {
  await signIn(page)
  await toPlanRow(page, 'Limits of the plan')

  const limits = page.getByRole('form', { name: 'Limits of the plan' })
  const cut = limits.getByRole('textbox', { name: /^Fastest loss in a cut, BMI under 30/ })
  await cut.fill('2')
  // A field of another group, far below, then Save.
  await limits.getByRole('button', { name: /^Autopilot/ }).click()
  await limits.getByRole('textbox', { name: /^Weigh-ins a step needs/ }).fill('4')
  await limits.getByRole('button', { name: 'Save' }).click()
  await expect(cut).toBeFocused()
  await expect(cut).toBeInViewport()
  await expect(cut).toHaveAttribute('aria-invalid', 'true')
  await expect(limits.getByText("Past the instance's limit, 1.")).toBeInViewport()
  await expect(page.getByText('Saved', { exact: true })).toBeHidden()

  // Fixed, it is saved; then as it was, for the other tests.
  await cut.fill('0.6')
  await limits.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Saved', { exact: true })).toBeVisible()
  expect(
    (
      await page.request.put('/api/v1/limits', {
        data: { limits: { cut_rate: -0.7, weigh_ins: 3 }, confirm: true },
      })
    ).ok(),
  ).toBe(true)
})
