import { useSyncExternalStore } from 'react'

// Theme is the look chosen in this browser: the system's (light or dark,
// as the device is set), or light or dark whatever the device says.
export type Theme = 'system' | 'light' | 'dark'

export const themes: readonly Theme[] = ['system', 'light', 'dark']

// The browser keeps the theme, as it does the language: it is the
// device's, not the user's (a phone dark at night, a laptop light).
const storageKey = 'tdeemon.theme'

function stored(): string | null {
  try {
    return globalThis.localStorage.getItem(storageKey)
  } catch {
    // Storage can be off (a private window, a policy): the system's then.
    return null
  }
}

// readTheme is the theme kept in this browser, else the system's.
export function readTheme(last: string | null = stored()): Theme {
  return last === 'light' || last === 'dark' ? last : 'system'
}

let current = readTheme()
const listeners = new Set<() => void>()

export function getTheme(): Theme {
  return current
}

// applyTheme puts a theme on the page. The tokens in index.css follow the
// class on <html>, light or dark, and with neither the system's
// (prefers-color-scheme), which needs no script, so a page that has not
// run one yet is already in the system's colours. The browser's bar
// takes the page's colour the same way: index.html has one for each
// scheme (theme-color), and a theme chosen makes its own the one that
// holds.
export function applyTheme(theme: Theme) {
  const root = globalThis.document.documentElement
  root.classList.toggle('light', theme === 'light')
  root.classList.toggle('dark', theme === 'dark')
  for (const meta of globalThis.document.querySelectorAll<HTMLMetaElement>(
    'meta[name="theme-color"][data-scheme]',
  )) {
    const scheme = meta.dataset.scheme ?? ''
    if (theme === 'system') {
      meta.media = `(prefers-color-scheme: ${scheme})`
    } else {
      meta.media = theme === scheme ? 'all' : 'not all'
    }
  }
}

// setTheme switches the theme and keeps it for the next visit.
export function setTheme(theme: Theme) {
  current = theme
  applyTheme(theme)
  try {
    globalThis.localStorage.setItem(storageKey, theme)
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

export function useTheme(): Theme {
  return useSyncExternalStore(subscribe, getTheme)
}

const darkQuery = '(prefers-color-scheme: dark)'

// The system's scheme, where the browser has a way to ask (jsdom has
// none).
function darkMedia(): MediaQueryList | undefined {
  return typeof globalThis.matchMedia === 'function' ? globalThis.matchMedia(darkQuery) : undefined
}

function systemDark(): boolean {
  return darkMedia()?.matches ?? false
}

function subscribeSystem(listener: () => void) {
  const media = darkMedia()
  media?.addEventListener('change', listener)
  return () => {
    media?.removeEventListener('change', listener)
  }
}

// useDark is whether the page is dark: the theme chosen, or the system's
// as it changes.
export function useDark(): boolean {
  const theme = useTheme()
  const system = useSyncExternalStore(subscribeSystem, systemDark)
  return theme === 'dark' || (theme === 'system' && system)
}
