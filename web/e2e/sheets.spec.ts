// The checks run code in the page, which has the DOM's types.
/// <reference lib="dom" />

import { expect, test, type Locator, type Page } from '@playwright/test'

import { signIn } from './user.ts'

// openLog opens the meal to log from the bar's Log a meal, as a phone
// does, and returns its sheet.
async function openLog(page: Page): Promise<Locator> {
  await signIn(page)
  await page
    .getByRole('navigation', { name: 'Sections' })
    .getByRole('button', { name: 'Log a meal' })
    .click()
  const sheet = page.getByRole('dialog', { name: 'New meal' })
  await expect(sheet).toBeVisible()
  await slidIn(sheet)
  return sheet
}

// slidIn waits until the sheet is in its place: a finger drags it from
// there.
async function slidIn(sheet: Locator) {
  await sheet.evaluate((el) => Promise.all(el.getAnimations().map((a) => a.finished)))
}

// dragDown drags a finger down from the top of the sheet, by its handle,
// through the touch events a phone sends, and lifts it.
async function dragDown(page: Page, sheet: Locator, by: number) {
  const box = await sheet.boundingBox()
  if (box === null) {
    throw new Error('no sheet to drag')
  }
  const x = box.x + box.width / 2
  const y = box.y + 8
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] })
  // Ten moves, each once the one before is sent, as a finger makes them.
  await Array.from({ length: 10 }, (_, i) => y + (by * (i + 1)) / 10).reduce<Promise<unknown>>(
    (before, to) =>
      before.then(() =>
        cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y: to }] }),
      ),
    Promise.resolve(),
  )
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
}

for (const motion of ['no-preference', 'reduce'] as const) {
  test.describe(`with ${motion} for motion`, () => {
    test.use({ reducedMotion: motion })

    test('on a phone a sheet is dragged down by its handle to close, the focus back on what opened it', async ({
      page,
    }) => {
      const sheet = await openLog(page)
      // The close button stays, for the keyboard and a screen reader.
      await expect(sheet.getByRole('button', { name: 'Close' })).toBeVisible()

      await dragDown(page, sheet, 400)

      await expect(sheet).toBeHidden()
      await expect(
        page
          .getByRole('navigation', { name: 'Sections' })
          .getByRole('button', { name: 'Log a meal' }),
      ).toBeFocused()
    })
  })
}

test.describe('on a desktop', () => {
  test.use({ viewport: { width: 1440, height: 900 }, isMobile: false, hasTouch: false })

  test('a sheet is dragged away to the right by its title', async ({ page }) => {
    await signIn(page)
    await page.keyboard.press('n')
    const sheet = page.getByRole('dialog', { name: 'New meal' })
    await expect(sheet).toBeVisible()
    await slidIn(sheet)
    const title = await sheet.getByRole('heading', { name: 'New meal' }).boundingBox()
    if (title === null) {
      throw new Error('no title to drag')
    }

    await page.mouse.move(title.x + title.width / 2, title.y + title.height / 2)
    await page.mouse.down()
    await page.mouse.move(title.x + title.width / 2 + 350, title.y + title.height / 2, {
      steps: 10,
    })
    await page.mouse.up()

    await expect(sheet).toBeHidden()
  })
})
