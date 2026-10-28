import { Combobox } from '@base-ui/react/combobox'
import { Check, ChevronsUpDown, LocateFixed } from 'lucide-react'
import { useId } from 'react'

import { Button } from '@/components/ui/button'
import { useMessages } from '@/i18n/messages'
import { fieldLabel, inputClass } from '@/lib/styles'
import { nameNow, renamed } from '@/lib/time-zones'
import { cn } from '@/lib/utils'

// zones are the time zones to choose from: the browser's own list under
// the names IANA gives them now, with UTC, the browser's zone and the
// user's stored one in it whatever it holds, an old name too (V8's has no
// UTC); undefined for a browser without a list, where the zone is typed,
// which the server checks.
function zones(stored: string): string[] | undefined {
  if (typeof Intl.supportedValuesOf !== 'function') {
    return undefined
  }
  return [
    ...new Set([...Intl.supportedValuesOf('timeZone').map(nameNow), 'UTC', ownZone(), stored]),
  ].toSorted()
}

// ownZone is the browser's time zone, under its name now.
function ownZone(): string {
  return nameNow(Intl.DateTimeFormat().resolvedOptions().timeZone)
}

// plain is a zone's name, or a search of one, as the search compares
// them: in lower case, a space for an underscore.
function plain(s: string): string {
  return s.toLocaleLowerCase().replaceAll('_', ' ').trim()
}

// matches tells a zone a search finds: by any part of its name, or of an
// old name of it (Kiev finds Europe/Kyiv).
function matches(zone: string, query: string): boolean {
  const q = plain(query)
  return [zone, ...Object.keys(renamed).filter((old) => renamed[old] === zone)].some((name) =>
    plain(name).includes(q),
  )
}

// TimeZoneField picks the user's time zone: a field to search the zones
// by name, which lists those it finds, and a button that takes the
// browser's own. A user's zone under an old name stays theirs until they
// pick another. A browser without the list of zones has a field to type
// one in.
export function TimeZoneField({
  value,
  stored,
  onChange,
}: {
  value: string
  // The user's zone as it is stored, which the list holds whatever it is.
  stored: string
  onChange: (zone: string) => void
}) {
  const t = useMessages()
  const id = useId()
  const choices = zones(stored)
  const own = ownZone()

  return (
    <div className="space-y-2">
      <label htmlFor={id} className={cn(fieldLabel, 'block')}>
        {t.settings.timeZone}
      </label>
      {choices ? (
        <Combobox.Root
          items={choices}
          value={value}
          filter={matches}
          onValueChange={(zone) => {
            if (zone !== null) {
              onChange(zone)
            }
          }}
        >
          <Combobox.InputGroup className="relative">
            <Combobox.Input
              id={id}
              autoComplete="off"
              spellCheck={false}
              className={cn(inputClass, 'pr-11')}
            />
            <Combobox.Trigger
              aria-label={t.settings.allZones}
              className="absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-r-md text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <ChevronsUpDown aria-hidden className="size-4" />
            </Combobox.Trigger>
          </Combobox.InputGroup>
          <Combobox.Portal>
            <Combobox.Positioner sideOffset={4} className="z-50 outline-none">
              <Combobox.Popup className="max-h-[min(20rem,var(--available-height))] w-(--anchor-width) max-w-(--available-width) overflow-y-auto overscroll-contain rounded-md border bg-popover py-1 text-popover-foreground shadow-sheet transition-opacity duration-(--duration-base) data-ending-style:opacity-0 data-starting-style:opacity-0">
                <Combobox.Empty className="block px-3.5 py-2.5 text-small text-muted-foreground empty:hidden">
                  {t.settings.noZone}
                </Combobox.Empty>
                <Combobox.List>
                  {(zone: string) => (
                    <Combobox.Item
                      key={zone}
                      value={zone}
                      className="flex min-h-11 cursor-default items-center gap-2 px-3.5 outline-none select-none data-highlighted:bg-muted lg:min-h-9"
                    >
                      <span className="min-w-0 flex-1 truncate">{zone}</span>
                      <Combobox.ItemIndicator className="text-primary">
                        <Check aria-hidden className="size-4" />
                      </Combobox.ItemIndicator>
                    </Combobox.Item>
                  )}
                </Combobox.List>
              </Combobox.Popup>
            </Combobox.Positioner>
          </Combobox.Portal>
        </Combobox.Root>
      ) : (
        <input
          id={id}
          value={value}
          autoComplete="off"
          spellCheck={false}
          placeholder="Europe/Zagreb"
          className={inputClass}
          onChange={(e) => {
            onChange(e.currentTarget.value.trim())
          }}
        />
      )}
      {/* Disabled where it would change nothing: the zone is the
          browser's already. */}
      <Button
        variant="outline"
        size="sm"
        disabled={value === own}
        onClick={() => {
          onChange(own)
        }}
      >
        <LocateFixed aria-hidden />
        {t.settings.detectZone}
      </Button>
    </div>
  )
}
