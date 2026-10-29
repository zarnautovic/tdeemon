import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { Toasts } from '@/components/toasts'
import { UpdatePrompt } from '@/components/update-prompt'
import { setLocale } from '@/i18n/locale'
import { toasts } from '@/lib/toast'

// The service worker's registration, as the PWA plugin's hook gives it:
// whether a new version waits, and what takes it.
const sw = vi.hoisted(() => ({
  ready: false,
  update: vi.fn<(reload?: boolean) => Promise<void>>(() => Promise.resolve()),
}))

vi.mock('virtual:pwa-register/react', () => ({
  useRegisterSW: () => ({
    needRefresh: [sw.ready, () => undefined],
    offlineReady: [false, () => undefined],
    updateServiceWorker: sw.update,
  }),
}))

afterEach(() => {
  cleanup()
  act(() => {
    toasts.close('update')
  })
  sw.ready = false
  sw.update.mockClear()
  setLocale('en')
})

test('a new version of the app waits for Reload, in a toast of its own', () => {
  sw.ready = true
  render(
    <>
      <Toasts />
      <UpdatePrompt />
    </>,
  )

  expect(screen.getByText('New version ready')).toBeDefined()
  expect(sw.update).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Reload' }))
  expect(sw.update).toHaveBeenCalledWith(true)
})

test('without a new version there is no toast, and in Croatian it says so in Croatian', () => {
  render(
    <>
      <Toasts />
      <UpdatePrompt />
    </>,
  )
  expect(screen.queryByText('New version ready')).toBeNull()
  cleanup()

  setLocale('hr')
  sw.ready = true
  render(
    <>
      <Toasts />
      <UpdatePrompt />
    </>,
  )
  expect(screen.getByText('Nova verzija je spremna')).toBeDefined()
  expect(screen.getByRole('button', { name: 'Osvježi' })).toBeDefined()
})
