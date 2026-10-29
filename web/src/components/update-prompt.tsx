import { useEffect } from 'react'
import { useRegisterSW } from 'virtual:pwa-register/react'

import { useMessages } from '@/i18n/messages'
import { sayUpdate } from '@/lib/toast'

// UpdatePrompt registers the app's service worker (vite.config.ts), which
// keeps its shell to open without a network, and says when a new version
// is ready: a toast whose Reload takes it, so the page is never reloaded
// under the user's hands, a meal half typed.
export function UpdatePrompt() {
  const t = useMessages()
  const {
    needRefresh: [ready],
    updateServiceWorker,
  } = useRegisterSW()

  useEffect(() => {
    if (ready) {
      sayUpdate(t.update.ready, {
        label: t.update.reload,
        run: () => void updateServiceWorker(true),
      })
    }
  }, [ready, t, updateServiceWorker])

  return null
}
