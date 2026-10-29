import type { components } from '@/api/schema'
import type { Day } from '@/api/types'
import { addDays } from '@/i18n/format'
import type { Locale } from '@/i18n/locale'

import { day, me, signedIn, today, type Handler } from './app'
import { planBody } from './plan'

type Summary = components['schemas']['SummaryBody']

// summaryOfNothing is calibration's summary of a user who measured
// nothing yet: no trend, no recovery, no TDEE; over is laid over it.
export function summaryOfNothing(over: Partial<Summary> = {}): Summary {
  return {
    date: today,
    incomplete_days: [],
    recovery: { flags: [] },
    tdee: {
      before_phase: false,
      doubts: ['few_days'],
      from: '2026-09-25',
      to: '2026-10-22',
      logged_days: 0,
      measured: false,
      trusted: false,
      weigh_ins: 0,
      needs: {
        days: { has: 0, need: 14 },
        logged_days: { has: 0, need: 7 },
        weigh_ins: { has: 0, need: 6 },
        weigh_in_days: { has: 0, need: 7 },
      },
      trust: { half_width: 200, weigh_ins: 3, logged_days: 4 },
    },
    verdict: {
      action: 'none',
      flags: [],
      gap_kcal: 0,
      guards: [],
      rate: 'none',
      reasons: ['no_estimate'],
      step_kcal: 0,
    },
    weeks: [],
    ...over,
  }
}

// weekOfNothing is the history of the week up to today, with nothing in
// any of its days.
export function weekOfNothing(): components['schemas']['HistoryBody'] {
  const from = addDays(today, -6)
  return {
    from,
    to: today,
    days: Array.from({ length: 7 }, (_, i) => ({
      date: addDays(from, i),
      status: i === 6 ? 'open' : 'complete',
      type: 'rest',
      meals: 0,
    })),
    weeks: [],
  }
}

// dayWithoutTarget is today, before any plan: nothing logged, no target.
export function dayWithoutTarget(over: Partial<Day> = {}): Day {
  const { target: _target, remaining: _remaining, ...rest } = day(over)
  return rest
}

// newUser are the routes of a user who has given nothing yet: no
// profile, no measurement, no meal, no access key and no plan; today
// has no target.
export function newUser(locale: Locale = 'en'): Record<string, Handler> {
  return {
    ...signedIn(locale),
    'GET /api/v1/me': () =>
      Response.json({
        ...me(),
        locale,
        profile: { missing: ['sex', 'birth_date', 'height_cm'] },
      }),
    'GET /api/v1/days/today': () => Response.json(dayWithoutTarget()),
    'GET /api/v1/summary': () => Response.json(summaryOfNothing()),
    'GET /api/v1/history': () => Response.json(weekOfNothing()),
    'GET /api/v1/plan': () => Response.json(planBody()),
    'GET /api/v1/tokens': () => Response.json({ tokens: [] }),
    'GET /api/v1/meals/recent': () => Response.json({ meals: [] }),
  }
}
