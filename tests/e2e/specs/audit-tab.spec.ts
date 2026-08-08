// Audit log: a host added via the API shows up in the safety tab's audit table.
//
// addHostHandler writes an AuditEntry {Action:"add", Mac, Hostname, ...}
// (internal/webapi/handlers_hosts.go:127). SafetyTab renders <AuditTab/>, whose table rows
// show entry.mac and entry.hostname verbatim (the action badge is
// i18n-translated via audit.action_<action>, so it is NOT a stable anchor).
//
// P3.6 hardening: seedHosts treats 409 (duplicate MAC) as success, so on a
// local re-run the host already exists, no new "add" audit entry is written
// for that seed, and a STALE row with the same MAC would match vacuously — a
// writeAudit no-op regression would slip through. Two pins fix this:
//   1. Clean slate: deleteHostApi(MAC) before seeding (404 is fine on a fresh
//      CI conf-dir) so the seed always creates the host fresh and writes a
//      new "add" audit entry THIS run.
//   2. Per-run-unique hostname: the hostname cell is NOT locale-dependent, so
//      matching MAC AND this run's hostname discriminates this run's entry
//      from any stale ones. If writeAudit is a no-op, no row with the
//      per-run hostname exists → spec fails.
//
// store.auditLog is populated by loadData() on dashboard mount, so the seed
// (written in beforeAll, before page load) is present when we open the tab.

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, deleteHostApi, CONF_DIR } from '../lib/api'

const MAC = 'aa:e1:00:00:00:01'
const AUDIT_FILE = `${CONF_DIR}/e2e-audit.conf`
// Per-run-unique hostname: digits-only suffix keeps it hostname-valid (the
// regex in internal/validate/validate.go requires alnum start/end), and pid+timestamp rules out
// collision with stale entries from a previous local run.
const HOSTNAME = `audit-${process.pid}-${Date.now()}`

test.beforeAll(async () => {
  const token = await apiLogin()
  // Clean slate: a leftover host with this MAC (prior local run) would make
  // the seed return 409 and skip writing a new audit entry. deleteHostApi
  // does not throw on 404, so this is safe on a fresh CI conf-dir too.
  await deleteHostApi(token, MAC, AUDIT_FILE)
  await seedHosts(token, [
    { mac: MAC, ip: '10.99.10.1', hostname: HOSTNAME, file: AUDIT_FILE },
  ])
})

test('audit: add-host entry appears in the safety-tab audit table', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Safety tab button is the only .btn-group member carrying the 🛡️ emoji
  // (the header also has 🛡️, but outside .btn-group).
  await page.locator('.btn-group button', { hasText: '🛡️' }).click()

  // Match MAC AND this run's unique hostname — pins that THIS run's "add"
  // audit entry was actually written (writeAudit regression → no such row).
  const row = page.locator('tbody tr', { hasText: MAC }).filter({ hasText: HOSTNAME })
  await expect(row).toBeVisible({ timeout: 10000 })
})
