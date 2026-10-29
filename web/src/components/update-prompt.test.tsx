import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { Toasts } from '@/components/toasts'
import { UpdatePrompt } from '@/components/update-prompt'
import { setLocale } from '@/i18n/locale'

// The browser's service worker as a built app registers it: a version
// active, and a new one waiting or not.
const sw = vi.hoisted(() => ({ waiting: null as EventTarget | null }))

function worker(postMessage: (message: unknown) => void = () => undefined) {
  return Object.assign(new EventTarget(), { state: 'installed', postMessage })
}

beforeEach(() => {
  vi.stubEnv('DEV', false)
  const active = worker()
  Object.defineProperty(navigator, 'serviceWorker', {
    configurable: true,
    value: {
      register: () =>
        Promise.resolve(
          Object.assign(new EventTarget(), {
            installing: null,
            waiting: sw.waiting,
            active,
            update: () => Promise.resolve(),
          }),
        ),
    },
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllEnvs()
  Reflect.deleteProperty(navigator, 'serviceWorker')
  sw.waiting = null
  setLocale('en')
})

async function renderPrompt() {
  render(
    <>
      <Toasts />
      <UpdatePrompt />
    </>,
  )
  await act(settled)
}

// The worker registers once a promise settles.
async function settled() {
  await new Promise((resolve) => {
    setTimeout(resolve, 0)
  })
}

test('a new version of the app waits for its Reload, in a toast of its own', async () => {
  const postMessage = vi.fn<(message: unknown) => void>()
  sw.waiting = worker(postMessage)
  await renderPrompt()

  expect(screen.getByText('New version ready')).toBeDefined()
  expect(postMessage).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Reload the app' }))
  expect(postMessage).toHaveBeenCalledWith({ type: 'SKIP_WAITING' })
})

test('without a new version there is no toast', async () => {
  await renderPrompt()
  expect(screen.queryByText('New version ready')).toBeNull()
})

test('in Croatian it says so in Croatian, and closed it does not come back in another language', async () => {
  setLocale('hr')
  sw.waiting = worker()
  await renderPrompt()

  expect(screen.getByText('Nova verzija je spremna')).toBeDefined()
  expect(screen.getByRole('button', { name: 'Osvježi aplikaciju' })).toBeDefined()
  // Hidden from a screen reader, as the toast is heard in the viewport.
  fireEvent.click(screen.getByLabelText('Zatvori'))
  await vi.waitFor(() => {
    expect(screen.queryByText('Nova verzija je spremna')).toBeNull()
  })
  await act(async () => {
    setLocale('en')
    await settled()
  })
  expect(screen.queryByText('New version ready')).toBeNull()
})
