import type { Day, Metric } from '@/api/types'
import { metricKinds } from '@/components/day/metric-kinds'

import { day as emptyDay, problem, today, type Handler } from './app'

// A request the day server got: its method, its path and its body.
export interface Sent {
  method: string
  path: string
  body: unknown
}

const units: Record<Metric['kind'], string> = {
  weight: 'kg',
  body_fat: '%',
  lean_mass: 'kg',
  steps: 'steps',
  active_kcal: 'kcal',
  basal_kcal: 'kcal',
  sleep_h: 'h',
  rhr: 'bpm',
  hrv: 'ms',
  vo2max: 'ml/kg/min',
  exercise_min: 'min',
}

// isObject tells a JSON object: the body the app sends, which its types
// hold to the API's.
function isObject(body: unknown): body is Record<string, unknown> {
  return typeof body === 'object' && body !== null
}

// checkedIn is day d with what a check-in says in place, as the server
// keeps it.
function checkedIn(d: Day, c: Record<string, unknown>): Day {
  const next: Day = { ...d }
  if (c.mood === 0) {
    delete next.mood
  } else if (typeof c.mood === 'number') {
    next.mood = c.mood
  }
  if (c.hunger === 0) {
    delete next.hunger
  } else if (typeof c.hunger === 'number') {
    next.hunger = c.hunger
  }
  if (typeof c.note === 'string') {
    next.note = c.note
  }
  if (c.training_day === 'plan') {
    delete next.training_toggle
    next.type = 'rest'
  } else if (c.training_day === 'training' || c.training_day === 'rest') {
    next.training_toggle = c.training_day === 'training'
    next.type = c.training_day
  }
  if (typeof c.excluded_reason === 'string') {
    next.status = c.excluded_reason === '' ? 'open' : 'excluded'
    next.excluded_reason = c.excluded_reason
  }
  return next
}

// dayServer is the server's routes for a day of the user's, today unless
// start says otherwise: it answers for it, takes check-ins and
// measurements as the API does, and keeps what it was sent.
export function dayServer(start: Day = emptyDay()) {
  let current = start
  const sent: Sent[] = []
  const path = `/api/v1/days/${start.date}`

  async function record(request: Request): Promise<unknown> {
    const body: unknown = request.method === 'DELETE' ? undefined : await request.json()
    sent.push({ method: request.method, path: new URL(request.url).pathname, body })
    return body
  }

  const routes: Record<string, Handler> = {
    [`GET ${path}`]: () => Response.json(current),
    ...(start.date === today ? { 'GET /api/v1/days/today': () => Response.json(current) } : {}),
    [`PATCH ${path}`]: async (request) => {
      const body = await record(request)
      // The server keeps no space at a note's ends (text.CheckLines).
      if (isObject(body) && typeof body.note === 'string' && body.note !== body.note.trim()) {
        return problem(422)
      }
      current = checkedIn(current, isObject(body) ? body : {})
      return Response.json(current)
    },
  }
  for (const kind of metricKinds) {
    routes[`PUT ${path}/metrics/${kind}`] = async (request) => {
      const body = await record(request)
      const value = isObject(body) && typeof body.value === 'number' ? body.value : Number.NaN
      const metric: Metric = { kind, value, unit: units[kind], source: 'manual' }
      const others = current.metrics.filter((m) => m.kind !== kind)
      current = { ...current, metrics: [...others, metric] }
      return Response.json(current)
    }
    routes[`DELETE ${path}/metrics/${kind}`] = async (request) => {
      await record(request)
      current = { ...current, metrics: current.metrics.filter((m) => m.kind !== kind) }
      return Response.json(current)
    }
  }
  return { routes, sent, day: () => current }
}
