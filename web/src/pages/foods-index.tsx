import { Apple } from 'lucide-react'

import { useMessages } from '@/i18n/messages'

// FoodsIndexPage is the pane beside the foods on a desktop before one is
// picked. A phone shows the foods instead.
export function FoodsIndexPage() {
  const t = useMessages()

  return (
    <div className="grid min-h-80 place-items-center rounded-lg border-2 border-dashed p-6 text-center">
      <div className="space-y-3">
        <Apple aria-hidden className="mx-auto size-8 text-muted-foreground" />
        <p className="max-w-xs text-muted-foreground">{t.foods.pick}</p>
      </div>
    </div>
  )
}
