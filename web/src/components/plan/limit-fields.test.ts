import { expect, test } from 'vitest'

import { limitsBody } from '@/test/plan'

import { check, limitFields, shown, typedValue } from './limit-fields'

// parse reads numbers as the English page does.
const parse = (s: string) => (/^\d+([.,]\d+)?$/.test(s) ? Number(s.replace(',', '.')) : undefined)

const field = (name: string) => {
  const f = limitFields.find((l) => l.name === name)
  if (!f) {
    throw new Error(`no limit ${name}`)
  }
  return f
}

test('every limit of the server has a field, and no other', () => {
  expect(limitFields.map((f) => f.name).toSorted()).toEqual(
    Object.keys(limitsBody().limits).toSorted(),
  )
})

test('a loss is shown and typed as how fast', () => {
  expect(shown(field('cut_rate'), -0.7)).toBe(0.7)
  expect(typedValue(field('cut_rate'), '0,5', parse)).toBe(-0.5)
  expect(Object.is(typedValue(field('cut_rate'), '0', parse), 0)).toBe(true)
  expect(shown(field('bulk_rate'), 0.25)).toBe(0.25)
  expect(typedValue(field('bulk_rate'), '0.3', parse)).toBe(0.3)
  expect(typedValue(field('bulk_rate'), '-0.3', parse)).toBeUndefined()
})

test('a signed limit is typed with a minus or without', () => {
  expect(typedValue(field('recomp_min'), '-0,4', parse)).toBe(-0.4)
  expect(typedValue(field('recomp_min'), ' −0.4 ', parse)).toBe(-0.4)
  expect(typedValue(field('recomp_min'), '0.1', parse)).toBe(0.1)
  expect(typedValue(field('recomp_min'), '-', parse)).toBeUndefined()
})

test('a value as it is changes nothing', () => {
  expect(check(limitsBody(), { cut_rate: '0.7', step_kcal: '100' }, parse)).toEqual({
    changes: {},
    loosened: [],
    problems: {},
  })
})

test('a tightened limit is a change, a loosened one is asked of the user too', () => {
  const c = check(
    limitsBody(),
    { cut_rate: '0.5', bulk_rate: '0.4', kcal_floor_male: '1600' },
    parse,
  )
  expect(c.changes).toEqual({ cut_rate: -0.5, bulk_rate: 0.4, kcal_floor_male: 1600 })
  expect(c.loosened).toEqual(['bulk_rate'])
  expect(c.problems).toEqual({})
  expect(check(limitsBody(), { cut_rate: '0.9' }, parse).loosened).toEqual(['cut_rate'])
})

test('a limit goes no further than the instance', () => {
  expect(check(limitsBody(), { cut_rate: '1.1', step_kcal: '200' }, parse).problems).toEqual({
    cut_rate: { kind: 'instance', value: -1 },
  })
  // The grams' floor is the user's only.
  expect(check(limitsBody(), { protein_floor_grams: '150' }, parse).problems).toEqual({})
})

test('a value of the wrong kind is no change', () => {
  const c = check(limitsBody(), { step_kcal: 'a lot', interval_days: '7.5' }, parse)
  expect(c.changes).toEqual({})
  expect(c.problems).toEqual({ step_kcal: { kind: 'number' }, interval_days: { kind: 'whole' } })
})

test('a share is of 0 to 100 %, a count of the days of a week of 0 to 7', () => {
  expect(check(limitsBody(), { fat_floor_pct: '101', weigh_ins: '8' }, parse).problems).toEqual({
    fat_floor_pct: { kind: 'range', min: 0, max: 100 },
    weigh_ins: { kind: 'range', min: 0, max: 7 },
  })
  expect(check(limitsBody(), { fat_floor_pct: '100', logged_days: '7' }, parse).problems).toEqual(
    {},
  )
})

test('a minimum stays within its maximum', () => {
  expect(check(limitsBody(), { recomp_min: '0.3' }, parse).problems).toEqual({
    recomp_min: { kind: 'order', other: 'recomp_max' },
  })
  expect(check(limitsBody(), { protein_ceiling: '1.5' }, parse).problems).toEqual({
    protein_ceiling: { kind: 'order', other: 'protein_floor' },
  })
})

test("a limit past the instance's other one is the user's to move, changed or not", () => {
  const lowered = limitsBody({ protein_floor: 2.8 })
  lowered.instance.protein_ceiling = 2.5
  expect(check(lowered, {}, parse).problems).toEqual({
    protein_floor: { kind: 'held', other: 'protein_ceiling', value: 2.5, to: 'lower' },
  })
  expect(check(lowered, { protein_floor: '2.4' }, parse).problems).toEqual({})

  const raised = limitsBody({ recomp_max: -0.3 })
  raised.instance.recomp_min = -0.2
  expect(check(raised, {}, parse).problems).toEqual({
    recomp_max: { kind: 'held', other: 'recomp_min', value: -0.2, to: 'raise' },
  })
})
