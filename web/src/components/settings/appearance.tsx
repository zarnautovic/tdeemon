import { useId } from 'react'

import { SegmentedControl } from '@/components/ui/segmented'
import { useMessages } from '@/i18n/messages'
import { setTheme, themes, useTheme } from '@/lib/theme'

// Appearance picks this browser's theme, the system's, light or dark, as
// segments of one control, which the page's title names. It takes effect
// at once and is not sent: a device's, not the user's.
export function Appearance() {
  const t = useMessages()
  const theme = useTheme()
  const id = useId()

  return (
    <SegmentedControl
      legend={t.settings.appearance}
      hideLegend
      options={themes.map((th) => ({ value: th, label: t.settings.themes[th] }))}
      value={theme}
      onChange={setTheme}
      describedBy={`${id}-hint`}
    >
      <p id={`${id}-hint`} className="text-sm text-muted-foreground">
        {t.settings.appearanceHint}
      </p>
    </SegmentedControl>
  )
}
