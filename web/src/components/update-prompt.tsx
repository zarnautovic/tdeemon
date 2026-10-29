import { useEffect, useEffectEvent } from 'react'

import { useMessages } from '@/i18n/messages'
import { watchUpdates } from '@/lib/service-worker'
import { sayUpdate } from '@/lib/toast'

// UpdatePrompt registers the app's service worker (lib/service-worker),
// which keeps its shell to open without a network, and says when a new
// version is ready: a toast whose Reload takes it, so the page is never
// reloaded under the user's hands, a meal half typed.
export function UpdatePrompt() {
  const t = useMessages()
  // Said in the language of the moment it comes, once: a toast closed
  // does not come back when the language changes.
  const say = useEffectEvent((take: () => void) => {
    sayUpdate(t.update.ready, { label: t.update.reload, run: take })
  })

  useEffect(() => {
    // The development server has no worker; the build writes it.
    if (import.meta.env.DEV) {
      return undefined
    }
    return watchUpdates((take) => {
      say(take)
    })
  }, [])

  return null
}
