// Audit log: a host added via the API shows up in the safety tab's audit table.
//
// addHostHandler writes an AuditEntry {Action:"add", Mac:<mac>, ...}
// (handlers_hosts.go). SafetyTab renders <AuditTab/>, whose table rows
// show entry.mac in a td.font-monospace. We match the seeded MAC rather
// than the (locale-dependent) action badge, so the assertion is stable
// under RU/EN.
//
// store.auditLog is populated by loadData() on dashboard mount, so the
// seed (written in beforeAll, before page load) is already present when
// we open the safety tab.

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const MAC = 'aa:e1:00:00:00:01'

test.beforeAll(async () => {
  const token = await apiLogin()
  await seedHosts(token, [
    { mac: MAC, ip: '10.99.10.1', hostname: 'audit-one', file: `${CONF_DIR}/e2e-audit.conf` },
  ])
})

test('audit: add-host entry appears in the safety-tab audit table', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Safety tab button is the only .btn-group member carrying the 🛡️ emoji
  // (the header also has 🛡️, but outside .btn-group).
  await page.locator('.btn-group button', { hasText: '🛡️' }).click()

  // The audit table is the only <tbody> rendered while the safety tab is
  // active (StaticView and others are v-if'd out).
  const row = page.locator('tbody tr', { hasText: MAC })
  await expect(row).toBeVisible({ timeout: 10000 })
})
