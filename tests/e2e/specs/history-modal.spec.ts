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
  const versionRow = modal.locator('tbody tr').first()
  await expect(versionRow).toBeVisible({ timeout: 10000 })

  // Diff vs current (≠ button) → pre.history-diff populated.
  await versionRow.locator('button.btn-outline-primary').click()
  await expect(modal.locator('pre.history-diff')).not.toBeEmpty()

  // Restore (⏪ in the row, confirm-guarded) → file reverts to version1.
  page.on('dialog', (d) => d.accept())
  await versionRow.locator('button.btn-outline-warning').click()

  await expect(page.locator('tbody tr', { hasText: GONE })).toHaveCount(0, { timeout: 10000 })
  await expect(page.locator('tbody tr', { hasText: KEEP })).toBeVisible()
})
