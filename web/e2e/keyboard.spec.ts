// A check runs code in the page, which has the DOM's types.
/// <reference lib="dom" />

import { expect, test, type Page } from '@playwright/test'

import { signIn } from './user.ts'

// The keyboard on a desktop: the skip to the content, the sections' keys,
// the undo of the latest change, and the day's meals as a list the arrows
// move through.
test.use({ viewport: { width: 1440, height: 900 }, isMobile: false, hasTouch: false })

// logMeal logs a meal of one estimate, through the API with the page's
// session, at a time ago (now by default), and returns its name, made up
// and of its own each run.
async function logMeal(page: Page, name: string, ago = 0): Promise<string> {
  const named = `${name} ${String(Date.now())}`
  const logged = await page.request.post('/api/v1/meals', {
    data: {
      description: named,
      eaten_at: new Date(Date.now() - ago).toISOString(),
      items: [{ name: named, nutrients: { kcal: 410, protein: 20, carbs: 50, fat: 12 } }],
    },
  })
  expect(logged.ok()).toBe(true)
  return named
}

// rowOf is a meal's row on the day, of its name: the button that opens it.
function rowOf(page: Page, name: string) {
  return page.getByRole('article', { name }).getByRole('button', { expanded: false }).first()
}

test('the first Tab shows the skip to the content, which takes the focus past the sections', async ({
  page,
}) => {
  await signIn(page)
  await page.goto('/foods')
  await expect(page.getByRole('heading', { name: 'Foods', level: 1 })).toBeVisible()

  await page.keyboard.press('Tab')
  const skip = page.getByRole('link', { name: 'Skip to content' })
  await expect(skip).toBeFocused()
  await expect(skip).toBeInViewport()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('main')).toBeFocused()
  await expect(page).toHaveURL('/foods')
  // The next stop is the page's, not a section's.
  await page.keyboard.press('Tab')
  expect(
    await page.evaluate(() => document.querySelector('main')?.contains(document.activeElement)),
  ).toBe(true)
})

test('G, then a letter, goes to each section, as the shortcuts list', async ({ page }) => {
  await signIn(page)
  // goes presses G and letter, and waits for the section's page.
  const goes = async (letter: string, path: string, heading: string) => {
    await page.keyboard.press('g')
    await page.keyboard.press(letter)
    await expect(page).toHaveURL(path)
    await expect(page.getByRole('heading', { name: heading, level: 1, exact: true })).toBeVisible()
  }
  await goes('f', '/foods', 'Foods')
  await goes('p', '/progress', 'Progress')
  await goes('l', '/plan', 'Plan')
  await goes('s', '/settings', 'Settings')
  await goes('d', '/', 'Today')
  await page.keyboard.press('?')
  const list = page.getByRole('dialog', { name: 'Keyboard shortcuts' })
  await expect(list.getByText('Undo the latest change')).toBeVisible()
  await expect(list.getByText('Repeat the meal today')).toBeVisible()
})

test('Ctrl+Z takes back the latest change, and says which', async ({ page }) => {
  await signIn(page)
  const name = await logMeal(page, 'Buckwheat groats')
  await page.goto('/')
  await expect(page.getByRole('article', { name })).toBeVisible()

  await page.keyboard.press('Control+z')

  await expect(page.getByText('Undone: a meal logged.')).toBeVisible()
  await expect(page.getByRole('article', { name })).toHaveCount(0)
})

test('the arrows move between the meals, Enter changes one, R repeats it and Delete deletes it', async ({
  page,
}) => {
  await signIn(page)
  // Two meals of today, a minute apart, the later the day's last; the
  // other tests' may be between them.
  const first = await logMeal(page, 'Rye crispbread', 60_000)
  const second = await logMeal(page, 'Barley salad')
  await page.goto('/')
  await expect(page.getByRole('article', { name: second })).toBeVisible()

  // The list is one stop of Tab: from the meals' heading, a row; the next
  // stop is past the list.
  await page.getByRole('heading', { name: 'Meals', exact: true }).click()
  await page.keyboard.press('Tab')
  expect(await page.evaluate(() => document.activeElement?.hasAttribute('data-meal'))).toBe(true)
  await page.keyboard.press('Tab')
  expect(await page.evaluate(() => document.activeElement?.hasAttribute('data-meal'))).toBe(false)

  // ↓ and ↑ move between the rows, End to the last.
  await rowOf(page, first).focus()
  await page.keyboard.press('ArrowDown')
  await expect(rowOf(page, first)).not.toBeFocused()
  expect(await page.evaluate(() => document.activeElement?.hasAttribute('data-meal'))).toBe(true)
  await page.keyboard.press('ArrowUp')
  await expect(rowOf(page, first)).toBeFocused()
  await page.keyboard.press('End')
  await expect(rowOf(page, second)).toBeFocused()
  await rowOf(page, first).focus()

  // Enter opens its editor, which gives the focus back to the row.
  await page.keyboard.press('Enter')
  const editor = page.getByRole('dialog', { name: 'Change the meal' })
  await expect(editor).toBeVisible()
  await expect(editor.getByLabel('What it was')).toHaveValue(first)
  await page.keyboard.press('Escape')
  await expect(editor).toBeHidden()
  await expect(rowOf(page, first)).toBeFocused()

  // R repeats it today: its new row takes the focus.
  await page.keyboard.press('r')
  await expect(page.getByText(`Repeated: ${first} · 410 kcal`)).toBeVisible()
  const repeats = page.getByRole('article', { name: first })
  await expect(repeats).toHaveCount(2)
  await expect(repeats.last().getByRole('button', { expanded: false }).first()).toBeFocused()

  // Delete deletes it, with the toast's Undo.
  await page.keyboard.press('Delete')
  await expect(page.getByText(`Deleted: ${first}`)).toBeVisible()
  await expect(repeats).toHaveCount(1)
  await page
    .getByRole('region', { name: 'Notifications' })
    .getByRole('button', { name: 'Undo' })
    .click()
  await expect(page.getByText('Undone: a meal deleted.')).toBeVisible()
  await expect(repeats).toHaveCount(2)
})

test('after N and a meal logged, the focus is on its row', async ({ page }) => {
  await signIn(page)
  // Eaten two days before at this hour: a card of the quick add's.
  const name = await logMeal(page, 'Millet bowl', 2 * 86_400_000)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Today', exact: true })).toBeVisible()

  await page.keyboard.press('n')
  const sheet = page.getByRole('dialog', { name: 'New meal' })
  await sheet.getByRole('button', { name: `Repeat: ${name}` }).click()

  await expect(sheet).toBeHidden()
  await expect(rowOf(page, name)).toBeFocused()
})
