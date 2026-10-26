import { expect, test, type Page } from '@playwright/test'

import { signIn } from './user.ts'

// propose gives the signed-in user what a plan needs (a profile and a
// weigh-in) and a proposal of a target from tomorrow, as a coach would
// make it through MCP; it returns the proposal's rationale.
async function propose(page: Page): Promise<string> {
  const api = page.request
  expect(
    (
      await api.put('/api/v1/profile', {
        data: { sex: 'male', birth_date: '1990-06-15', height_cm: 180 },
      })
    ).ok(),
  ).toBe(true)
  expect((await api.put('/api/v1/days/today/metrics/weight', { data: { value: 85 } })).ok()).toBe(
    true,
  )
  const me: unknown = await (await api.get('/api/v1/me')).json()
  const today = typeof me === 'object' && me !== null && 'today' in me ? String(me.today) : ''
  const tomorrow = new Date(`${today}T00:00:00Z`)
  tomorrow.setUTCDate(tomorrow.getUTCDate() + 1)
  const rationale = 'A first target, made up for the end-to-end tests.'
  const made = await api.post('/api/v1/proposals', {
    data: {
      rationale,
      model: 'claude-opus-5-5',
      targets: [
        {
          from: tomorrow.toISOString().slice(0, 10),
          training: { kcal: 2100, protein: 150, fat: 60 },
        },
      ],
    },
  })
  expect(made.status()).toBe(201)
  return rationale
}

test('a proposed change is accepted in the app, and taken back from its history', async ({
  page,
}) => {
  await signIn(page)
  const rationale = await propose(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('link', { name: 'Settings' })
    .click()
  await page.getByRole('link', { name: /^Plan/ }).click()

  const card = page.getByRole('region', { name: 'A proposed change' })
  await expect(card.getByText(rationale)).toBeVisible()
  await expect(card.getByText('Proposed by claude-opus-5-5')).toBeVisible()
  await expect(card.getByText(/^New target from/)).toBeVisible()
  await card.getByRole('button', { name: 'Accept' }).click()
  await expect(card).toBeHidden()

  const history = page.getByRole('region', { name: 'History of the plan' })
  const revision = history.getByRole('listitem').filter({ hasText: rationale })
  await expect(revision).toBeVisible()
  await revision.getByRole('button', { name: 'Take back' }).click()
  await revision.getByRole('button', { name: 'Take back' }).click()
  await expect(revision.getByText(/Taken back$/)).toBeVisible()

  // What the plan needed cleared again, for the other tests.
  expect((await page.request.put('/api/v1/profile', { data: {} })).ok()).toBe(true)
  expect((await page.request.delete('/api/v1/days/today/metrics/weight')).ok()).toBe(true)
})
