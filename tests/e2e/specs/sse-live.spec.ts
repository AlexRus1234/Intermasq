// SSE live. Two variants:
//  1) simplified — /api/events streams under Bearer auth (200 +
//     text/event-stream). Always runs.
//  2) full — append an ARP entry to the writable -arp-file mid-test and
//     observe a new 🟢 via the SSE delta. Needs the e2e instance started
//     with a WRITABLE -arp-file (CI copies tests/fixtures/arp-sample.txt to
//     /tmp/e2e-arp.txt and exports ARP_FILE). Without ARP_FILE the full
//     variant skips.
//
// /api/events is behind authMiddleware (auth.go) which accepts only a
// Bearer JWT / X-API-Key (?token= was removed — it leaked JWTs into logs).
// Native EventSource can't send headers, so the simplified variant uses
// fetch() from the page context: fetch resolves as soon as status + headers
// arrive, letting us assert 200 + text/event-stream without consuming the
// (long-lived) body.
//
// SSE broadcaster (sse.go:78) polls the arp file every 5s and pushes a
// delta only when the set changes; the full variant therefore waits up to
// 20s (4 cycles) for the new 🟢.

import { test, expect } from '@playwright/test'
import { appendFileSync } from 'node:fs'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const ARP_FILE = process.env.ARP_FILE

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

test('sse-live: appended ARP entry surfaces as new online dot', async ({ page }) => {
  test.skip(!ARP_FILE, 'needs ARP_FILE env (writable -arp-file)')

  // MAC absent from arp-sample → initially 🔴. Unique prefix to avoid
  // colliding with other specs (arp-sample uses 11:22:33:44:55:0X /
  // aa:bb:cc:dd:ee:01).
  const NEW_MAC = '99:88:77:66:55:01'

  const token = await apiLogin()
  await seedHosts(token, [
    {
      mac: NEW_MAC,
      ip: '10.99.99.1',
      hostname: 'sse-target',
      file: `${CONF_DIR}/e2e-sse.conf`,
    },
  ])

  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Start assert: our row shows 🔴 (offline) — NEW_MAC is not in the arp table.
  const offlineDot = page.locator('tr', { hasText: NEW_MAC }).locator('span.text-muted')
  await expect(offlineDot).toBeVisible({ timeout: 15000 })

  // Mutate the writable arp file: append NEW_MAC flagged 0x2 (reachable).
  appendFileSync(ARP_FILE!, `10.99.99.1     0x1         0x2         ${NEW_MAC}     *        eth0\n`)

  // SSE broadcaster polls every 5s (sse.go:78); wait up to 20s for 🟢.
  const onlineDot = page.locator('tr', { hasText: NEW_MAC }).locator('span.text-success')
  await expect(onlineDot).toBeVisible({ timeout: 20000 })
})
