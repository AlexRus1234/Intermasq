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

// History modal: seed two hosts in one file (2nd write snapshots version1 =
// single-host state), open the 🕒 modal, confirm a version is listed, show a
// diff vs current, then restore that version → the second host disappears.
//
// restoreHistoryVersion() runs `dnsmasq --test` (passes on the CI stock
// config) and uses confirm() in the UI. The modal emits 'restored' which
// triggers actions.loadData().

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const FILE = `${CONF_DIR}/e2e-history.conf`
const KEEP = 'aa:d1:00:00:00:01'
const GONE = 'aa:d1:00:00:00:02'

test.beforeAll(async () => {
  const token = await apiLogin()
  await seedHosts(token, [
    { mac: KEEP, ip: '10.99.60.11', hostname: 'keep', file: FILE },
    { mac: GONE, ip: '10.99.60.12', hostname: 'gone-after-restore', file: FILE },
  ])
})

test('history modal: list version, diff, restore', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  await page.locator('.nav-link', { hasText: 'e2e-history.conf' }).click()

  // 🕒 opens the history modal (rendered only when a file is selected).
  await page.locator('.btn-outline-secondary', { hasText: '🕒' }).click()
  const modal = page.locator('.modal-content')
  await expect(modal).toBeVisible({ timeout: 5000 })

  // A version row must be present.
  // P2.11: pin the row by its data-version identity, not by DOM position.
  // The history list is newest-first (internal/dnsmasq/history.go); `.first()` would
  // silently pick the wrong version if that order ever flips or if the
  // version set grows. We want the OLDEST snapshot — the one taken before
  // GONE was added, whose content is {KEEP} only — so restoring it makes
  // GONE disappear. Version stamps are YYYYMMDD-HHMMSS(-NN)?, so
  // lexicographic string-min == oldest, independent of render order.
  const versionRows = modal.locator('tbody tr[data-version]')
  await expect(versionRows.first()).toBeVisible({ timeout: 10000 })
  const versions = await versionRows.evaluateAll((rows) =>
    rows.map((r) => r.getAttribute('data-version') || '').filter(Boolean)
  )
  expect(versions.length, 'at least one history version present').toBeGreaterThan(0)
  const oldest = versions.reduce((a, b) => (a < b ? a : b))
  const versionRow = modal.locator(`tbody tr[data-version="${oldest}"]`)
  await expect(versionRow).toBeVisible()

  // Diff vs current (≠ button) → pre.history-diff populated.
  await versionRow.locator('button.btn-outline-primary').click()
  await expect(modal.locator('pre.history-diff')).not.toBeEmpty()

  // Restore (⏪ in the row, confirm-guarded) → file reverts to version1.
  page.on('dialog', (d) => d.accept())
  await versionRow.locator('button.btn-outline-warning').click()

  await expect(page.locator('tbody tr', { hasText: GONE })).toHaveCount(0, { timeout: 10000 })
  await expect(page.locator('tbody tr', { hasText: KEEP })).toBeVisible()
})
