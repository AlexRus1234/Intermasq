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

// .bak rollback via the UI: seed two hosts in one file (the second write
// leaves a .bak = the single-host state), select that file's tab so the ⏪
// button appears (v-if selectedFile!=='all' && hasBackup), confirm-rollback,
// and the second host must disappear while the first stays. rollbackFile()
// uses confirm(); restoreBackup reloads store.hosts.

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const FILE = `${CONF_DIR}/e2e-rollback.conf`
const KEEP = 'aa:c1:00:00:00:01'
const GONE = 'aa:c1:00:00:00:02'

test.beforeAll(async () => {
  const token = await apiLogin()
  await seedHosts(token, [
    { mac: KEEP, ip: '10.99.50.11', hostname: 'keep', file: FILE },
    { mac: GONE, ip: '10.99.50.12', hostname: 'gone-after-rollback', file: FILE },
  ])
})

test('rollback reverts file to .bak state', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Select the file's tab so the per-file ⏪ button renders.
  await page.locator('.nav-link', { hasText: 'e2e-rollback.conf' }).click()

  await expect(page.locator('tbody tr', { hasText: KEEP })).toBeVisible({ timeout: 10000 })
  await expect(page.locator('tbody tr', { hasText: GONE })).toBeVisible()

  page.on('dialog', (d) => d.accept())
  await page.locator('.btn-outline-warning', { hasText: '⏪' }).click()

  // .bak held the single-host state → GONE removed, KEEP remains.
  await expect(page.locator('tbody tr', { hasText: GONE })).toHaveCount(0, { timeout: 10000 })
  await expect(page.locator('tbody tr', { hasText: KEEP })).toBeVisible()
})
