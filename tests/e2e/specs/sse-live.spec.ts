// SSE live (simplified). The full variant — mutate the arp fixture mid-test
// and observe a new 🟢 via the SSE delta — needs the e2e instance started
// with a WRITABLE -arp-file (tests/fixtures/arp-sample.txt is read-only),
// which is a CI-infrastructure change inside the opt-in L4 step
// (see логи/gap2-finish.md for the full-variant plan). Until that's added, this spec covers the
// simplified alternative the same section allows: the SSE endpoint itself
// is live and streams under auth.
//
// /api/events is behind authMiddleware (auth.go) which accepts only a
// Bearer JWT / X-API-Key (?token= was removed — it leaked JWTs into logs).
// Native EventSource can't send headers, so we use fetch() from the page
// context: fetch resolves as soon as status + headers arrive, letting us
// assert 200 + text/event-stream without consuming the (long-lived) body.

import { test, expect } from '@playwright/test'

test('sse-live: /api/events streams under Bearer auth', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const result = await page.evaluate(async () => {
    const token = localStorage.getItem('token')
    if (!token) return { error: 'no token in localStorage' }
    const r = await fetch('/api/events', { headers: { Authorization: `Bearer ${token}` } })
    return { status: r.status, contentType: r.headers.get('content-type') || '' }
  })

  expect(result.status).toBe(200)
  expect(result.contentType).toContain('text/event-stream')
})
