import { useSyncExternalStore } from 'react'

// The browser keeps whether a user put their first steps off for later,
// as it does the theme: the card leaves today's page until the plan's
// page shows it again. A browser that keeps nothing (a private window, a
// policy) keeps it for this visit.
const storageKey = (user: string) => `tdeemon.firstSteps.later.${user}`

const visit = new Set<string>()
const listeners = new Set<() => void>()

function isLater(user: string): boolean {
  try {
    return globalThis.localStorage.getItem(storageKey(user)) === 'true'
  } catch {
    return visit.has(user)
  }
}

// putOff puts the user's first steps off for later, or brings them back.
export function putOff(user: string, later: boolean) {
  try {
    if (later) {
      globalThis.localStorage.setItem(storageKey(user), 'true')
    } else {
      globalThis.localStorage.removeItem(storageKey(user))
    }
  } catch {
    if (later) {
      visit.add(user)
    } else {
      visit.delete(user)
    }
  }
  for (const listener of listeners) {
    listener()
  }
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

// usePutOff tells whether the user, once known, put their first steps
// off for later.
export function usePutOff(user: string | undefined): boolean {
  return useSyncExternalStore(subscribe, () => user !== undefined && isLater(user))
}
