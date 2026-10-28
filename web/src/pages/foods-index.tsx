import { Apple } from 'lucide-react'

import { useMessages } from '@/i18n/messages'

// FoodsIndexPage is the pane beside the foods on a desktop before one is
// picked: a line that says to pick one, no taller than it needs, beside
// the list's top. A phone shows the foods instead.
export function FoodsIndexPage() {
  const t = useMessages()

  return (
    <div className="flex items-center gap-3 rounded-lg border border-dashed px-4 py-3.5">
      <Apple aria-hidden className="size-5 shrink-0 text-muted-foreground" />
      <p className="text-small text-muted-foreground">{t.foods.pick}</p>
    </div>
  )
}
