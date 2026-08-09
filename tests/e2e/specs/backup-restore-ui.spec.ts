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

// ZIP backup + restore via the UI.
//  - backup download: 💾 triggers a blob download whose suggested filename
//    is the hardcoded "dnsmasq_backup.zip" (api/system.js).
//  - restore: 📤 opens a dynamic <input type=file> (→ filechooser event),
//    confirm(), then restoreBackup() POSTs /backup/restore. We capture a
//    real zip via the API in beforeAll (state with R1), delete R1, then
//    restore → R1 must reappear. restoreBackupZip MERGES (only overwrites
//    files present in the zip), so other specs' files are untouched.

import { test, expect, request } from '@playwright/test'
import { writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { apiLogin, seedHosts, deleteHostApi, CONF_DIR, BASE_URL } from '../lib/api'

const MAC = 'aa:d2:00:00:00:01'
const FILE = `${CONF_DIR}/e2e-restore.conf`
const ZIP_PATH = join(tmpdir(), 'intermasq-e2e-backup.zip')

test.beforeAll(async () => {
  const token = await apiLogin()
  await seedHosts(token, [
    { mac: MAC, ip: '10.99.40.11', hostname: 'restore-me', file: FILE },
  ])

  // Capture a backup zip (contains e2e-restore.conf with R1) to a Node
  // temp path the filechooser can hand to the browser.
  const ctx = await request.newContext({ baseURL: BASE_URL })
  const res = await ctx.get('/api/backup', { headers: { Authorization: `Bearer ${token}` } })
  expect(res.ok(), `backup fetch failed: ${res.status()}`).toBeTruthy()
  writeFileSync(ZIP_PATH, await res.body())
  await ctx.dispose()

  // Delete R1 so the restore test starts with R1 absent.
  await deleteHostApi(token, MAC, FILE)
})

test('backup download triggers with expected filename', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const [download] = await Promise.all([
    page.waitForEvent('download'),
    page.locator('.btn-outline-info').click(),
  ])
  expect(download.suggestedFilename()).toBe('dnsmasq_backup.zip')
})

test('restore from zip brings back the deleted host', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  // Precondition: R1 is gone.
  await expect(page.locator('tbody tr', { hasText: MAC })).toHaveCount(0)

  // 📤 creates a dynamic file input → filechooser; then confirm(); then
  // restoreBackup() which alerts on success (accepted) and reloads store.
  page.on('filechooser', async (c) => c.setFiles(ZIP_PATH))
  page.on('dialog', (d) => d.accept())
  await page.locator('.btn-outline-warning', { hasText: '📤' }).click()

  await expect(page.locator('tbody tr', { hasText: MAC })).toBeVisible({ timeout: 15000 })
})
