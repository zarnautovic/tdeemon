import { Pencil, Plus } from 'lucide-react'
import { useRef, useState } from 'react'

import type { components } from '@/api/schema'
import { PhaseForm } from '@/components/plan/phase-form'
import { Sheet } from '@/components/sheet'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

type Plan = components['schemas']['PlanBody']
type Phase = components['schemas']['PhaseBody']

// EditPhase is the button that edits a phase of the plan in a sheet, or,
// without one, plans the next phase.
export function EditPhase({ plan, phase }: { plan: Plan; phase?: Phase | undefined }) {
  const t = useMessages()
  const [open, setOpen] = useState(false)
  const opener = useRef<HTMLButtonElement>(null)
  const e = t.plan.edit
  const title = phase === undefined ? e.next : e.phase

  return (
    <>
      <Button
        ref={opener}
        variant="outline"
        size="sm"
        onClick={() => {
          setOpen(true)
        }}
      >
        {phase === undefined ? <Plus aria-hidden /> : <Pencil aria-hidden />}
        {title}
      </Button>
      <Sheet open={open} onOpenChange={setOpen} title={title} finalFocus={opener}>
        <PhaseForm
          plan={plan}
          phase={phase}
          onDone={() => {
            setOpen(false)
          }}
        />
      </Sheet>
    </>
  )
}
