// A1 regression: sorting the hosts table must not change the number of
// rendered rows for our seeded prefix.
//
// Root cause of A1 (логи/duis.md): HostTable.vue uses `:key="h.mac"` which
// is not guaranteed unique, so on re-sort Vue can reuse DOM and rows appear
// duplicated. For this guard the mechanism is irrelevant — the invariant
// is "row count for our prefix is stable across sort clicks".
//
// We seed 5 hosts with a unique prefix and count ONLY matching rows so
// leftover hosts from other specs (which share the conf-dir) can't skew
// the count.

import { test, expect, request } from '@playwright/test'

const BASE_URL = process.env.E2E_BASE_URL || 'http://localhost:18083'
const ADMIN_USER = process.env.ADMIN_USER || 'admin'
const ADMIN_PASS = process.env.ADMIN_PASS || 'pass1234'
const CONF_DIR = process.env.CONF_DIR!

const PREFIX = 'aa:11:11:11:11'

test.beforeAll(async () => {
  const ctx = await request.newContext({ baseURL: BASE_URL })
  const r = await ctx.post('/api/login', {
    data: { username: ADMIN_USER, password: ADMIN_PASS },
  })
  const { token } = await r.json()

  const file = `${CONF_DIR}/e2e-sort.conf`
  for (let i = 1; i <= 5; i++) {
    const mac = `${PREFIX}:${String(i).padStart(2, '0')}`
    const res = await ctx.post('/api/hosts', {
      data: { mac, ip: `10.99.${i}.2`, hostname: `sort${i}`, file },
      headers: { Authorization: `Bearer ${token}` },
    })
    // 200 = created, 409 = already exists (repeat run). Anything else is fatal.
    if (res.status() !== 200 && res.status() !== 409) {
      throw new Error(`seed ${mac} failed: ${res.status()} ${await res.text()}`)
    }
  }
  await ctx.dispose()
})

test('A1 regression: sorting does not change row count', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const rows = page.locator('tbody tr td.font-monospace', { hasText: PREFIX })
  await expect(rows).toHaveCount(5, { timeout: 15000 })

  for (let i = 0; i < 3; i++) {
    await page.locator('th', { hasText: 'IP' }).click()
    await expect(rows).toHaveCount(5)
  }
})
