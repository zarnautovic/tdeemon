import { useEffect, useId, useRef } from 'react'

import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

// Loosen asks the user to loosen what a change loosens: the focus goes
// to Back, the safe answer, and the question describes both answers.
export function Loosen({
  names,
  pending,
  onYes,
  onBack,
}: {
  names: string
  pending: boolean
  onYes: () => void
  onBack: () => void
}) {
  const t = useMessages()
  const id = useId()
  const back = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    back.current?.focus()
  }, [])

  return (
    <div className="space-y-2 rounded-lg border border-amber-500/50 p-3 text-sm">
      <p id={id}>{t.plan.loosen(names)}</p>
      <div className="flex gap-2">
        <Button variant="destructive" aria-describedby={id} disabled={pending} onClick={onYes}>
          {t.plan.yesLoosen}
        </Button>
        {/* Not while the loosening is sent: it may be taken all the same. */}
        <Button
          ref={back}
          variant="ghost"
          aria-describedby={id}
          disabled={pending}
          onClick={onBack}
        >
          {t.plan.keep}
        </Button>
      </div>
    </div>
  )
}
