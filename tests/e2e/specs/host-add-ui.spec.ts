// Add a host via the HostForm UI (single-add mode) and confirm it shows up
// in the table without a full page reload (HostForm.saveHost →
// actions.loadData() → Vue re-renders).
//
// Selectors:
//  - MAC input has a HARDCODED placeholder "MAC (aa:bb...)" (HostForm.vue)
//    — not i18n, so it's a stable anchor.
//  - The file input is the one grouped with the Add (btn-success) button.
//  - The Add button is the only .btn-success on the static tab.

import { test, expect } from '@playwright/test'
import { CONF_DIR } from '../lib/api'

const MAC = 'aa:33:44:55:66:01'

test('add host via form appears in table without reload', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  await page.locator('input[placeholder="MAC (aa:bb...)"]').fill(MAC)

  // Explicit file: don't rely on the default (store.hosts[0]?.file), which
  // couples this spec to whatever other specs seeded before it.
  const fileInput = page.locator('.input-group:has(.btn-success) input.form-control')
  await fileInput.fill(`${CONF_DIR}/e2e-add.conf`)

  await page.locator('.btn-success').click()

  const row = page.locator('tbody tr', { hasText: MAC })
  await expect(row).toBeVisible({ timeout: 10000 })
})
