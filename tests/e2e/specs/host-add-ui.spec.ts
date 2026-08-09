// Intermasq - Web panel for dnsmasq
// Copyright (C) 2026 AlexRus1234
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

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
