import { useState } from 'react'

import { useLogMetric } from '@/api/day'
import { statusOf } from '@/api/errors'
import { units, type MetricKind } from '@/components/day/metric-kinds'
import { useFormat } from '@/i18n/format'
import { useMessages } from '@/i18n/messages'
import limits from '@/lib/limits.json'
import { sayChange } from '@/lib/toast'

// useMetricValue stores a measurement of a kind for day date, as typed in
// the language and the kind's unit. What is not a number, or not a value
// of the kind (limits.json, the server's bounds), it says so of (problem),
// and sends nothing, until the field changes (clear). Saved, a toast says
// so, by the kind's name, with Undo; onSaved is told then.
export function useMetricValue(date: string, kind: MetricKind, onSaved?: () => void) {
  const t = useMessages()
  const f = useFormat()
  const log = useLogMetric(date, () => {
    sayChange(
      { title: t.toast.savedOf(t.day.metric[kind]) },
      { last: { action: 'metric.set', entity: `${date}/${kind}` }, date },
    )
  })
  const [wrong, setWrong] = useState<'number' | 'outside' | undefined>(undefined)
  const { least, most } = limits.metrics[kind]
  // A value the server refuses (422) is out of what the kind can be:
  // sending it again would not help.
  const refused = log.isError && statusOf(log.error) === 422

  function save(value: number) {
    log.mutate(
      { params: { path: { date, kind } }, body: { value } },
      { onSuccess: () => onSaved?.() },
    )
  }

  let problem: string | undefined
  if (wrong === 'number') {
    problem = t.day.notANumber
  } else if (wrong === 'outside') {
    problem = t.day.between(f.number(least, 1), f.number(most, 1), units[kind])
  } else if (refused) {
    problem = t.day.outOfRange
  }

  return {
    log,
    problem,
    invalid: wrong !== undefined || log.isError,
    // give saves what was typed, unless it is wrong.
    give: (typed: string) => {
      const value = f.parse(typed)
      if (value === undefined) {
        setWrong('number')
      } else if (value < least || value > most) {
        setWrong('outside')
      } else {
        setWrong(undefined)
        save(value)
      }
    },
    // retry sends again the value that did not reach the server.
    retry: () => {
      if (log.variables) {
        save(log.variables.body.value)
      }
    },
    // clear forgets what was wrong, as the field changes.
    clear: () => {
      setWrong(undefined)
      if (log.isError) {
        log.reset()
      }
    },
  }
}
