// The checks of a meal's lines run code in the page, which has the DOM's
// types.
/// <reference lib="dom" />

import { expect, test } from '@playwright/test'

import { openMeal, signIn } from './user.ts'

test('the day shows what was logged, and the days before it', async ({ page }) => {
  await signIn(page)
  // Logged as a model would, through the API, with the page's session.
  const meal = await page.request.post('/api/v1/meals', {
    data: {
      description: 'Lunch',
      items: [
        { name: 'Chicken with rice', nutrients: { kcal: 640, protein: 45, carbs: 70, fat: 15 } },
      ],
    },
  })
  expect(meal.ok()).toBe(true)
  const weighing = await page.request.put('/api/v1/days/today/metrics/weight', {
    data: { value: 72.5 },
  })
  expect(weighing.ok()).toBe(true)

  await page.goto('/')

  const lunch = page.getByRole('article').filter({ hasText: 'Lunch' })
  await expect(lunch).toContainText('640 kcal')
  await openMeal(lunch)
  await expect(lunch).toContainText('Chicken with rice')
  await expect(page.getByRole('region', { name: 'Body today' })).toContainText('72.5 kg')

  await page.getByRole('link', { name: 'The day before' }).click()
  await expect(page.getByText('Nothing logged yet.')).toBeVisible()
  await expect(page.getByRole('region', { name: 'Body', exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Back to today' }).click()
  await expect(page.getByRole('heading', { name: 'Today', exact: true })).toBeVisible()
})

test("another day's date is on a line between the arrows, the way back to today under it", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await signIn(page)
  await page.getByRole('link', { name: 'The day before' }).click()

  const date = page.getByRole('heading', { level: 1 })
  const back = page.getByRole('link', { name: 'Back to today' })
  await expect(back).toBeVisible()
  const [before, title, after, pill, week] = await Promise.all([
    page.getByRole('link', { name: 'The day before' }).boundingBox(),
    date.boundingBox(),
    page.getByRole('link', { name: 'The day after' }).boundingBox(),
    back.boundingBox(),
    page.getByRole('navigation', { name: 'The week' }).boundingBox(),
  ])
  if (!before || !title || !after || !pill || !week) {
    throw new Error('a part of the header is not shown')
  }
  // The date's text on one line.
  const lines = await date.evaluate((h) => {
    const range = document.createRange()
    range.selectNodeContents(h)
    return new Set(Array.from(range.getClientRects(), (r) => Math.round(r.top))).size
  })
  expect(lines).toBe(1)
  // The arrows either side of it, the way back under it, above the week.
  expect(before.x + before.width).toBeLessThanOrEqual(title.x)
  expect(after.x).toBeGreaterThanOrEqual(title.x + title.width)
  expect(pill.y).toBeGreaterThan(title.y + title.height)
  expect(pill.y + pill.height).toBeLessThanOrEqual(week.y)
})

test("a meal's row gives its name the room, and its values stay with their labels", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await signIn(page)
  const long = 'Grilled chicken with roasted vegetables, rice and a yoghurt sauce'
  const logged = await Promise.all(
    (
      [
        ['Yoghurt, granola, honey', 386],
        [long, 1180],
      ] as const
    ).map(([description, kcal]) =>
      page.request.post('/api/v1/meals', {
        data: {
          description,
          items: [
            {
              name: 'A wholegrain sourdough loaf with sunflower seeds, from the bakery',
              nutrients: { kcal, protein: 47.2, carbs: 0.4, fat: 5.4 },
            },
          ],
        },
      }),
    ),
  )
  expect(logged.every((r) => r.ok())).toBe(true)

  await page.goto('/')
  const meal = (description: string) => page.getByRole('article').filter({ hasText: description })
  // A name of a few words is whole: nothing of it is cut.
  const name = meal('Yoghurt, granola, honey').getByText('Yoghurt, granola, honey', {
    exact: true,
  })
  expect(await name.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBe(true)
  // The time beside it, and not over it.
  const time = await meal('Yoghurt, granola, honey').locator('time').boundingBox()
  const named = await name.boundingBox()
  expect((time?.x ?? 0) + (time?.width ?? 0)).toBeLessThanOrEqual(named?.x ?? 0)
  // The energy on one line, as high as a short meal's, which has room.
  const energy = (description: string, kcal: string) =>
    meal(description).getByText(kcal, { exact: true })
  const short = await energy('Yoghurt, granola, honey', '386').boundingBox()
  const beside = await energy(long, '1,180').boundingBox()
  expect(short).not.toBeNull()
  expect(beside?.height).toBe(short?.height)

  // An item of a long name leaves its values little room: they break
  // between pairs, never within one ("F" on a line, "5.4" on the next).
  await openMeal(meal(long))
  const values = meal(long).getByText(/^1,180\skcal\s·/)
  const lines = await values.evaluate((e) => {
    const text = e.firstChild
    if (!(text instanceof Text)) {
      return []
    }
    // Each character's line, by the top of its box.
    const range = document.createRange()
    const byTop = new Map<number, string>()
    for (let i = 0; i < text.length; i++) {
      range.setStart(text, i)
      range.setEnd(text, i + 1)
      const top = Math.round(range.getBoundingClientRect().top)
      byTop.set(top, (byTop.get(top) ?? '') + (text.data[i] ?? ''))
    }
    return Array.from(byTop.values(), (line) => line.replaceAll('\u00a0', ' ').trim())
  })
  expect(lines.length).toBeGreaterThan(1)
  const pair = String.raw`(?:[\d,.]+ kcal|[PCF] [\d,.]+)`
  for (const line of lines) {
    expect(line).toMatch(new RegExp(`^${pair}(?: · ${pair})*(?: ·)?$`))
  }
})
