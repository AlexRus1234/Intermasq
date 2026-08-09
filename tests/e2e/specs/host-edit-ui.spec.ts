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

// Edit an existing host via the UI: click the row's ✏️ → HostForm switches
// to edit mode (Save becomes .btn-warning) → change the IP → Save → the
// row's IP cell updates without a full page reload (HostForm.saveHost does
// delete-then-add, then actions.loadData()).

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const MAC = 'aa:88:11:22:33:01'
const NEW_IP = '10.99.10.99'

test.beforeAll(async () => {
  const token = await apiLogin()
  await seedHosts(token, [
    { mac: MAC, ip: '10.99.10.21', hostname: 'editme', file: `${CONF_DIR}/e2e-edit.conf` },
  ])
})

test('edit host via UI updates the row without reload', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const row = page.locator('tbody tr', { hasText: MAC })
  await expect(row).toBeVisible({ timeout: 15000 })

  // Per-row edit button (✏️) — scoped to the row so it can't be confused
  // with the bulk-edit button in the .bg-danger bar.
  await row.locator('button.btn-outline-secondary').click()

  // Edit mode confirmed by the Save button turning .btn-warning. Scope to
  // .input-group so it can't be confused with the toolbar's "🔄 Apply"
  // button, which is also .btn-warning.
  await expect(page.locator('.input-group .btn-warning')).toBeVisible({ timeout: 5000 })

  // The IP input is the one grouped with the 🎲 auto-IP button.
  const ipInput = page.locator('.input-group:has(button:has-text("🎲")) input.form-control')
  await ipInput.fill(NEW_IP)
  await page.locator('.input-group .btn-warning').click()

  // IP cell must reflect the new IP.
  await expect(row.locator('td.fw-bold.text-primary')).toHaveText(NEW_IP, { timeout: 10000 })
})
