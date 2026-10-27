import { useId } from 'react'

import { useMessages } from '@/i18n/messages'
import { setTheme, themes, useTheme } from '@/lib/theme'

// Appearance picks this browser's theme, the system's, light or dark, as
// segments of one control. It takes effect at once and is not sent: a
// device's, not the user's.
export function Appearance() {
  const t = useMessages()
  const theme = useTheme()
  const id = useId()

  return (
    <fieldset className="space-y-2" aria-describedby={`${id}-hint`}>
      <legend className="font-medium">{t.settings.appearance}</legend>
      <div className="flex gap-1 rounded-md bg-surface-2 p-1">
        {themes.map((th) => (
          <label
            key={th}
            className="flex h-9 flex-1 cursor-pointer items-center justify-center rounded-sm text-sm text-muted-foreground transition-colors has-checked:bg-card has-checked:font-semibold has-checked:text-foreground has-checked:shadow-card has-focus-visible:ring-2 has-focus-visible:ring-ring"
          >
            <input
              type="radio"
              name={`${id}-theme`}
              className="sr-only"
              checked={theme === th}
              onChange={() => {
                setTheme(th)
              }}
            />
            {t.settings.themes[th]}
          </label>
        ))}
      </div>
      <p id={`${id}-hint`} className="text-sm text-muted-foreground">
        {t.settings.appearanceHint}
      </p>
    </fieldset>
  )
}
