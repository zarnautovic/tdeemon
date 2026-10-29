import { expect, test } from '@playwright/test'

import { signIn } from './user.ts'

// The other tests block the service worker (playwright.config.ts); these
// are about it.
test.use({ serviceWorkers: 'allow' })

test('the app installs: its manifest, its icons and the icon iOS takes', async ({
  page,
  request,
}) => {
  await page.goto('/login')
  const href = await page.locator('link[rel="manifest"]').getAttribute('href')
  expect(href).toBe('/manifest.webmanifest')
  const answer = await request.get('/manifest.webmanifest')
  expect(answer.headers()['content-type']).toBe('application/manifest+json')
  const manifest: unknown = await answer.json()
  const icons = ['/pwa-192.png', '/pwa-512.png', '/maskable-512.png']
  expect(manifest).toMatchObject({
    name: 'tdeemon',
    display: 'standalone',
    start_url: '/',
    icons: icons.map((src) =>
      src.startsWith('/maskable') ? { src, purpose: 'maskable' } : { src },
    ),
  })

  const touch = await page.locator('link[rel="apple-touch-icon"]').getAttribute('href')
  const answers = await Promise.all([...icons, touch ?? ''].map((src) => request.get(src)))
  for (const icon of answers) {
    expect(icon.ok(), icon.url()).toBe(true)
    expect(icon.headers()['content-type']).toBe('image/png')
  }
})

test('once opened, the app opens without a network, and says the server is out of reach', async ({
  page,
  context,
}) => {
  await signIn(page)
  await page.evaluate(async () => {
    await navigator.serviceWorker.ready
  })
  // The worker takes the page from its next load.
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Today', exact: true })).toBeVisible()
  expect(await page.evaluate(() => navigator.serviceWorker.controller !== null)).toBe(true)

  await context.setOffline(true)
  try {
    await page.reload()
    // The app's own page from the worker, not the browser's: it says
    // what it cannot reach.
    await expect(page.getByText('The server cannot be reached.')).toBeVisible()
  } finally {
    await context.setOffline(false)
  }
})
