// Add a DNS A-record via the UI on the DNS tab. Exercises AliasForm: type
// stays A (default), domain + target typed, the reactive <code> preview
// must show address=/<domain>/<target>, and after Add the AliasTable row
// appears. addAlias() has NO alert on success (api/dns.js), so no dialog
// handler is needed on the happy path.
//
// All form inputs are scoped to the AliasForm card (.card.p-3.shadow-sm):
// the dashboard search box (App.vue) is also a .form-control and lives in
// the toolbar outside the card, so unscoped positional selectors would hit
// it first.

import { test, expect } from '@playwright/test'
import { CONF_DIR } from '../lib/api'

const DOMAIN = 'e2ea.lan'
const TARGET = '10.99.99.10'

test('add DNS A-record via UI renders preview + row', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Switch to the DNS tab (🌐 in the tab bar).
  await page.locator('.btn-group button', { hasText: '🌐' }).click()

  const form = page.locator('.card.p-3.shadow-sm')

  // domain / target are the first two standalone inputs in the card.
  await form.locator('input.form-control').nth(0).fill(DOMAIN)
  await form.locator('input.form-control').nth(1).fill(TARGET)

  // File is the input grouped with the Add button (default file is empty
  // on a fresh alias store → saveAlias would refuse without it).
  await form.locator('.input-group:has(.btn-success) input.form-control').fill(`${CONF_DIR}/e2e-alias.conf`)

  // Reactive directive preview must reflect the A-record.
  await expect(form.locator('code')).toContainText(`address=/${DOMAIN}/${TARGET}`)

  // Add → AliasTable row appears (store.aliases refresh).
  await form.locator('.input-group .btn-success').click()
  const row = page.locator('tbody tr', { hasText: DOMAIN })
  await expect(row).toBeVisible({ timeout: 10000 })
})
