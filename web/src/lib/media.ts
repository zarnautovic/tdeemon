import { useSyncExternalStore } from 'react'

// A desktop's width, from which the app has its sidebar: Tailwind's lg.
const desktopQuery = '(min-width: 64rem)'

function desktopMedia(): MediaQueryList | undefined {
  return typeof globalThis.matchMedia === 'function'
    ? globalThis.matchMedia(desktopQuery)
    : undefined
}

function subscribe(onChange: () => void): () => void {
  const media = desktopMedia()
  media?.addEventListener('change', onChange)
  return () => {
    media?.removeEventListener('change', onChange)
  }
}

// useDesktop is whether the window is a desktop's width now, as lg: is,
// for what a page lays out other than by its classes (two panes, or one
// of them). Without matchMedia it is a phone's.
export function useDesktop(): boolean {
  return useSyncExternalStore(subscribe, () => desktopMedia()?.matches ?? false)
}

// touchScreen tells a screen a finger points at, (pointer: coarse), where
// a text field given the focus opens the keyboard over half the screen.
// Without matchMedia it is not one.
export function touchScreen(): boolean {
  return typeof globalThis.matchMedia === 'function'
    ? globalThis.matchMedia('(pointer: coarse)').matches
    : false
}
