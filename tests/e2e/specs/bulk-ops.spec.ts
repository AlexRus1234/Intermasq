// Bulk operations via the UI: select rows with checkboxes, then
//   - bulk-move (📦): move hosts to another .conf file
//   - bulk-edit (✏️): IP prefix transform (10.99.70 → 10.99.71)
//
// The bulk action bar (HostTable) only renders when >=1 row is selected.
// It lives in a .bg-danger container with three .btn-light buttons
// distinguished by emoji (📦 move / ✏️ edit / 🗑️ delete). Each opens its
// own modal. We scope emoji selectors to ".bg-danger .btn-group" so the
// per-row edit button (also ✏️) can't shadow the bulk button.
//
// NOTE: this is a UI-functional spec for the bulk bar + modals, NOT an A5
// regression — see логи/gap2-playwright-bootstrap.md (A5 needs separate
// reproduction before it can be honestly regression-tested).

import { test, expect } from '@playwright/test'
import { apiLogin, seedHosts, CONF_DIR } from '../lib/api'

const MOVE_PREFIX = 'aa:66:77:88:99'
const EDIT_PREFIX = 'aa:77:88:99:aa'

test.beforeAll(async () => {
  const token = await apiLogin()
  await seedHosts(token, [
    { mac: `${MOVE_PREFIX}:01`, ip: '10.99.90.11', hostname: 'move-one', file: `${CONF_DIR}/e2e-bulk-a.conf` },
    { mac: `${MOVE_PREFIX}:02`, ip: '10.99.90.12', hostname: 'move-two', file: `${CONF_DIR}/e2e-bulk-a.conf` },
    { mac: `${EDIT_PREFIX}:01`, ip: '10.99.70.21', hostname: 'edit-one', file: `${CONF_DIR}/e2e-bulk-edit.conf` },
    { mac: `${EDIT_PREFIX}:02`, ip: '10.99.70.22', hostname: 'edit-two', file: `${CONF_DIR}/e2e-bulk-edit.conf` },
  ])
})

// Select every row matching one of the given MACs.
async function selectRows(page: import('@playwright/test').Page, macs: string[]) {
  for (const mac of macs) {
    await page.locator('tbody tr', { hasText: mac }).locator('input.form-check-input').check()
  }
}

test('bulk-move: selected hosts move to another file', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  await selectRows(page, [`${MOVE_PREFIX}:01`, `${MOVE_PREFIX}:02`])

  await page.locator('.bg-danger .btn-group button', { hasText: '📦' }).click()
  const modal = page.locator('.modal-content')
  await expect(modal).toBeVisible({ timeout: 5000 })

  // The move modal's file <select> only lists files that already have
  // hosts; e2e-bulk-b.conf is empty so it's not listed. Use the custom
  // target input instead.
  await modal.locator('input.form-control').fill(`${CONF_DIR}/e2e-bulk-b.conf`)
  await modal.locator('.btn-primary').click()

  // In "all files" view each row shows its file cell; the moved host's
  // cell must now show the new filename.
  const row = page.locator('tbody tr', { hasText: `${MOVE_PREFIX}:01` })
  await expect(row.locator('td.small.text-muted')).toHaveText(/e2e-bulk-b\.conf/, { timeout: 10000 })
})

test('bulk-edit: IP prefix transform changes IPs', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  await selectRows(page, [`${EDIT_PREFIX}:01`, `${EDIT_PREFIX}:02`])

  await page.locator('.bg-danger .btn-group button', { hasText: '✏️' }).click()
  const modal = page.locator('.modal-content')
  await expect(modal).toBeVisible({ timeout: 5000 })

  // old/new IP prefix are the first two inputs in the edit modal.
  await modal.locator('input.form-control').nth(0).fill('10.99.70')
  await modal.locator('input.form-control').nth(1).fill('10.99.71')
  await modal.locator('.btn-warning').click()

  // IP cell (td.fw-bold.text-primary) must reflect the transformed IP.
  const row = page.locator('tbody tr', { hasText: `${EDIT_PREFIX}:01` })
  await expect(row.locator('td.fw-bold.text-primary')).toHaveText(/10\.99\.71\.21/, { timeout: 10000 })
})
