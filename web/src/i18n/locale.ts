import { useSyncExternalStore } from 'react'

import type { components } from '@/api/schema'

// The languages the app speaks are the API's (a user's locale), so the two
// cannot drift: a new one in the API fails the type check here until it
// is added, with its messages.
export type Locale = components['schemas']['UserBody']['locale']

const known: Record<Locale, true> = { en: true, hr: true }

export const baseLocale: Locale = 'en'

export function isLocale(s: string): s is Locale {
  return Object.hasOwn(known, s)
}

// The browser keeps the last language, so the sign-in page, before the
// app knows the user, speaks the one they chose.
const storageKey = 'tdeemon.locale'

function stored(): string | null {
  try {
    return globalThis.localStorage.getItem(storageKey)
  } catch {
    // Storage can be off (a private window, a policy): the browser's
    // languages decide then.
    return null
  }
}

// detectLocale picks the language before the app knows the user: the one
// chosen last in this browser, else the first of the browser's languages
// the app speaks (hr-HR is hr), else the base language.
export function detectLocale(
  last: string | null = stored(),
  preferred: readonly string[] = globalThis.navigator.languages,
): Locale {
  if (last !== null && isLocale(last)) {
    return last
  }
  for (const tag of preferred) {
    const lang = tag.toLowerCase().split('-')[0] ?? ''
    if (isLocale(lang)) {
      return lang
    }
  }
  return baseLocale
}

let current = detectLocale()
const listeners = new Set<() => void>()

export function getLocale(): Locale {
  return current
}

// setLocale switches the app's language and keeps it for the next visit;
// every component that reads messages renders again.
export function setLocale(locale: Locale) {
  if (locale === current) {
    return
  }
  current = locale
  try {
    globalThis.localStorage.setItem(storageKey, locale)
  } catch {
    // Kept for this visit only.
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

export function useLocale(): Locale {
  return useSyncExternalStore(subscribe, getLocale)
}
