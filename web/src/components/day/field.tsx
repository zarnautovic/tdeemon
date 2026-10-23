import type { ReactNode } from 'react'

import { fieldLabel } from '@/lib/styles'

// Field is a part of the day a person changes: its label, where its last
// change stands next to it, and its control below. The label is for the
// control htmlFor names, or, with labelId, names a fieldset of buttons.
export function Field({
  label,
  htmlFor,
  labelId,
  status,
  children,
}: {
  label: string
  htmlFor?: string
  labelId?: string
  status: ReactNode
  children: ReactNode
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-baseline justify-between gap-x-2">
        {htmlFor === undefined ? (
          <span id={labelId} className={fieldLabel}>
            {label}
          </span>
        ) : (
          <label htmlFor={htmlFor} className={fieldLabel}>
            {label}
          </label>
        )}
        {status}
      </div>
      {children}
    </div>
  )
}
