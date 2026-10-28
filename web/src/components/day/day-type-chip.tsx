import { Menu } from '@base-ui/react/menu'
import { Check, ChevronDown } from 'lucide-react'

import { useCheckIn } from '@/api/day'
import type { Day } from '@/api/types'
import { SaveStatus } from '@/components/day/save-status'
import { useMessages } from '@/i18n/messages'
import { sayChange } from '@/lib/toast'
import { cn } from '@/lib/utils'

const choices = ['training', 'rest', 'plan'] as const

type Choice = (typeof choices)[number]

// choiceOf is the day's type as the user chose it: plan when the plan's
// training weekdays decide.
function choiceOf(day: Day): Choice {
  if (day.training_toggle === undefined) {
    return 'plan'
  }
  return day.training_toggle ? 'training' : 'rest'
}

// DayTypeChip is the day's type, which picks its target, as a chip in the
// day's header; its menu sets it: training, rest, or as the plan has it,
// which says what the plan decided (a workout the day had, or a training
// weekday, is training). A toast says it is saved, with Undo.
export function DayTypeChip({ day }: { day: Day }) {
  const t = useMessages()
  const change = useCheckIn(day.date, () => {
    sayChange(
      { title: t.toast.dayType },
      { last: { action: 'day.checkin', entity: day.date }, date: day.date },
    )
  })
  const sent = change.variables?.body.training_day
  const chosen = (change.isPending || change.isError) && sent !== undefined ? sent : choiceOf(day)
  // The type the day has, or will once the choice on its way is saved:
  // the plan's is the server's.
  const type = chosen === 'plan' ? day.type : chosen
  const name = type === 'training' ? t.day.training : t.day.rest

  return (
    <div className="flex flex-col items-end gap-1">
      <Menu.Root>
        <Menu.Trigger
          aria-label={t.day.dayTypeOf(name)}
          className={cn(
            "relative flex h-9 shrink-0 items-center gap-1.5 rounded-full border px-3 text-small font-semibold whitespace-nowrap outline-none after:absolute after:inset-x-0 after:-inset-y-1 after:content-[''] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
            type === 'training'
              ? 'border-primary bg-primary-soft text-primary'
              : 'border-border bg-card text-foreground',
          )}
        >
          {name}
          <ChevronDown aria-hidden className="size-3.5" />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Positioner sideOffset={6} align="end" className="z-40">
            <Menu.Popup className="min-w-48 rounded-lg border bg-card p-1 shadow-sheet outline-none">
              <Menu.RadioGroup
                value={chosen}
                onValueChange={(value: Choice) => {
                  if (value !== chosen) {
                    change.mutate({
                      params: { path: { date: day.date } },
                      body: { training_day: value },
                    })
                  }
                }}
              >
                {choices.map((choice) => (
                  <Menu.RadioItem
                    key={choice}
                    value={choice}
                    closeOnClick
                    className="flex h-11 cursor-default items-center gap-2 rounded-md pr-3 pl-2 text-sm outline-none select-none data-highlighted:bg-muted"
                  >
                    <span className="flex size-4 items-center justify-center">
                      <Menu.RadioItemIndicator>
                        <Check aria-hidden className="size-4" />
                      </Menu.RadioItemIndicator>
                    </span>
                    {t.day.dayTypes[choice]}
                    {choice === 'plan' && choiceOf(day) === 'plan' && (
                      // Read out as "By plan · rest", the space outside.
                      <>
                        {' '}
                        <span className="text-muted-foreground">
                          · {t.day.planDecided[day.type]}
                        </span>
                      </>
                    )}
                  </Menu.RadioItem>
                ))}
              </Menu.RadioGroup>
            </Menu.Popup>
          </Menu.Positioner>
        </Menu.Portal>
      </Menu.Root>
      <SaveStatus change={change} />
    </div>
  )
}
