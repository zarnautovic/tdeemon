import { expect, test } from 'vitest'

import { HttpError, NoAnswerError, retry, statusOf } from '@/api/errors'

test("a failure's status is the answer's, or its problem's", () => {
  expect(statusOf(new HttpError(401))).toBe(401)
  expect(statusOf({ status: 422, title: 'Unprocessable Entity' })).toBe(422)
  expect(statusOf({ status: '401' })).toBeUndefined()
  expect(statusOf(new NoAnswerError(new TypeError('Failed to fetch')))).toBeUndefined()
  expect(statusOf(null)).toBeUndefined()
})

test('a query is asked again once, after no answer or a server error', () => {
  expect(retry(0, new NoAnswerError(new TypeError('Failed to fetch')))).toBe(true)
  expect(retry(0, new HttpError(502))).toBe(true)
  expect(retry(0, { status: 500 })).toBe(true)
  expect(retry(1, new NoAnswerError(new TypeError('Failed to fetch')))).toBe(false)
  expect(retry(1, new HttpError(503))).toBe(false)
})

test('a refusal, or a failure of the app itself, is not asked again', () => {
  expect(retry(0, new HttpError(401))).toBe(false)
  expect(retry(0, { status: 404 })).toBe(false)
  expect(retry(0, new Error('a bug'))).toBe(false)
  expect(retry(0, new TypeError("Cannot read properties of undefined (reading 'x')"))).toBe(false)
})
