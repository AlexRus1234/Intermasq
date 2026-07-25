// Host delete via UI: a seeded host is visible, clicking the row's ✕
// (after accepting the native confirm()) removes the row WITHOUT a full
// page reload — HostTable.deleteHost calls actions.loadData() which
// re-fetches /api/hosts and Vue re-renders.

import { test, expect, request } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL || 'http://localhost:18083'
const ADMIN_USER = process.env.ADMIN_USER || 'admin'
const ADMIN_PASS = process.env.ADMIN_PASS || 'pass1234'
const CONF_DIR = process.env.CONF_DIR!

const MAC = 'aa:22:33:44:55:01'

test.beforeAll(async () => {
  const ctx = await request.newContext({ baseURL: BASE_URL })
  const r = await ctx.post('/api/login', {
    data: { username: ADMIN_USER, password: ADMIN_PASS },
  })
  const { token } = await r.json()

  const file = `${CONF_DIR}/e2e-crud.conf`
  const res = await ctx.post('/api/hosts', {
    data: { mac: MAC, ip: '10.99.50.2', hostname: 'crudseed', file },
    headers: { Authorization: `Bearer ${token}` },
  })
  if (res.status() !== 200 && res.status() !== 409) {
    throw new Error(`seed ${MAC} failed: ${res.status()} ${await res.text()}`)
  }
  await ctx.dispose()
})

test('delete host via UI removes row without reload', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const row = page.locator('tbody tr', { hasText: MAC })
  await expect(row).toBeVisible({ timeout: 15000 })

  // HostTable.deleteHost uses the browser's confirm(); Playwright dismisses
  // dialogs by default, which would silently cancel the delete. Accept first.
  page.on('dialog', (d) => d.accept())
  await row.locator('button.btn-outline-danger').click()

  await expect(row).toBeHidden({ timeout: 10000 })
})
