import { expect, type Page } from '@playwright/test'

// listTools calls /mcp as an AI coach does, with its access key: the
// tools it may use (tools/list), which it expects to name get_day.
export async function listTools(page: Page, key: string) {
  const answer = await page.request.post('/mcp', {
    headers: {
      Authorization: `Bearer ${key}`,
      Accept: 'application/json, text/event-stream',
    },
    data: { jsonrpc: '2.0', id: 1, method: 'tools/list' },
  })
  expect(answer.ok()).toBe(true)
  expect(await answer.text()).toContain('"get_day"')
}
