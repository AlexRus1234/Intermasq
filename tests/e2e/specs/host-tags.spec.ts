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
