// Reload via the toolbar "🔄 Apply": applyConfig() POSTs /api/reload and
// always pops an alert (success or error). We accept the alert and assert
// the response is 200 — that means `dnsmasq --test` passed and the init
// caller's restart succeeded (with -init-system=none the restart is a
// no-op, per checklist §12.3 / smoke). Response status is the clean
// success gate since the alert text is locale-dependent.

import { test, expect } from '@playwright/test'

test('reload via Apply returns 200', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // applyConfig() alerts on both success and error; accept unconditionally.
  page.on('dialog', (d) => d.accept())

  const [resp] = await Promise.all([
    page.waitForResponse((r) => r.url().includes('/api/reload') && r.status() === 200),
    page.locator('.btn-warning', { hasText: '🔄' }).click(),
  ])
  expect(resp.status()).toBe(200)
})
