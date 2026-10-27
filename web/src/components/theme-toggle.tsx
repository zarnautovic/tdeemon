import { Moon, Sun } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { setTheme, useDark } from '@/lib/theme'

// ThemeToggle switches the page to the other of the themes it shows,
// light or dark, whichever chose it (the system's included). It is a
// toggle of the dark theme: pressed when the page is dark.
export function ThemeToggle() {
  const t = useMessages()
  const dark = useDark()

  return (
    <Button
      variant="secondary"
      size="icon-sm"
      aria-label={t.nav.darkTheme}
      aria-pressed={dark}
      onClick={() => {
        setTheme(dark ? 'light' : 'dark')
      }}
    >
      {dark ? <Sun aria-hidden /> : <Moon aria-hidden />}
    </Button>
  )
}
