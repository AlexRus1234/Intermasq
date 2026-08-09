// Add a host with DHCP assignment tags (set:iot, set:guest) and confirm them.
// render as badges in the host row. Exercises the full parse → API →
// render path for tags (validated client-side and server-side).

import { test, expect } from '@playwright/test'
import { CONF_DIR } from '../lib/api'

const MAC = 'aa:44:55:66:77:01'

test('host with tags renders badges', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  await page.locator('input[placeholder="MAC (aa:bb...)"]').fill(MAC)

  // Target the tags input by its stable placeholder (same in ru/en). It is
  // no longer the only .font-monospace input in the row — the lease_time
  // field shares that class — so the old class-only selector is ambiguous.
  await page
    .locator('input[placeholder="set:iot,set:guest"]')
    .fill('set:iot,set:guest')

  await page.locator('.input-group:has(.btn-success) input.form-control').fill(`${CONF_DIR}/e2e-tags.conf`)
  await page.locator('.btn-success').click()

  const row = page.locator('tbody tr', { hasText: MAC })
  await expect(row).toBeVisible({ timeout: 10000 })
  await expect(row.locator('span.badge', { hasText: 'set:iot' })).toBeVisible()
  await expect(row.locator('span.badge', { hasText: 'set:guest' })).toBeVisible()
})
