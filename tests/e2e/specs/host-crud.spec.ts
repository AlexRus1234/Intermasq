// Host delete via UI: a seeded host is visible, clicking the row's ✕
// (after accepting the native confirm()) removes the row WITHOUT a full
// page reload — HostTable.deleteHost calls actions.loadData() which
// re-fetches /api/hosts and Vue re-renders.

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const MAC = 'aa:22:33:44:55:01'

test.beforeAll(async () => {
  const token = await apiLogin()
  await seedHosts(token, [
    { mac: MAC, ip: '10.99.50.2', hostname: 'crudseed', file: `${CONF_DIR}/e2e-crud.conf` },
  ])
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
