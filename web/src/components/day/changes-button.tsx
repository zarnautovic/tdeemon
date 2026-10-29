import { History } from 'lucide-react'
import { useState } from 'react'

import { RecentChanges } from '@/components/recent-changes'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// ChangesButton opens the user's recent changes (RecentChanges) from the
// day's header: a change of the day is taken back there after its toast
// is gone, and what an AI coach did shows. Closed, the focus comes back
// to it.
export function ChangesButton() {
  const t = useMessages()
  const [open, setOpen] = useState(false)

  return (
    <>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t.changes.title}
        onClick={() => {
          setOpen(true)
        }}
      >
        <History aria-hidden />
      </Button>
      <RecentChanges open={open} onOpenChange={setOpen} />
    </>
  )
}
