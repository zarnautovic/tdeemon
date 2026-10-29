import { defineConfig, devices } from '@playwright/test'

import { newUser, user } from './e2e/user.ts'

// The end-to-end tests run the app in Chromium as a phone sees it, against
// the real server: the binary built with the web app (make e2e builds
// it), over a fresh database with the tests' user, and a new one for the
// first steps (scripts/e2e-server.sh).
const port = 18_080

export default defineConfig({
  testDir: 'e2e',
  // One at a time: the tests share one user, and the server's limit of
  // failed sign-ins, ten in 15 minutes for an account or an address.
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${String(port)}`,
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'phone', use: { ...devices['Pixel 7'] } }],
  webServer: {
    command: `../scripts/e2e-server.sh ../.e2e/tdeemon ${String(port)} ../.e2e/data`,
    url: `http://127.0.0.1:${String(port)}/healthz`,
    env: { E2E_EMAIL: user.email, E2E_PASSWORD: user.password, E2E_NEW_EMAIL: newUser.email },
    reuseExistingServer: false,
  },
})
