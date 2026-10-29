import { Pencil } from 'lucide-react'
import { useRef, useState } from 'react'

import type { components } from '@/api/schema'
import { TargetsForm } from '@/components/plan/targets-form'
import { Sheet } from '@/components/sheet'
import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'

type Plan = components['schemas']['PlanBody']

// EditTargets is the button that edits the targets by type of day, from
// today on, in a sheet.
export function EditTargets({ plan }: { plan: Plan }) {
  const t = useMessages()
  const [open, setOpen] = useState(false)
  const opener = useRef<HTMLButtonElement>(null)
  const e = t.plan.edit

  return (
    <>
      <Button
        ref={opener}
        variant="ghost"
        size="sm"
        aria-label={e.targetsLabel}
        onClick={() => {
          setOpen(true)
        }}
      >
        <Pencil aria-hidden />
        {e.targets}
      </Button>
      <Sheet open={open} onOpenChange={setOpen} title={e.targetsTitle} finalFocus={opener}>
        <TargetsForm
          plan={plan}
          onDone={() => {
            setOpen(false)
          }}
        />
      </Sheet>
    </>
  )
}
