// Bulk text import via HostForm's text mode: paste 3 "MAC IP hostname"
// lines, the client-side parsed count must read 3, Import writes them,
// and three rows appear in the static hosts table. saveBulkHosts toasts
// on success (no alert()), so no dialog handler.

import { test, expect } from '@playwright/test'
import { CONF_DIR } from '../lib/api'

const PREFIX = 'aa:b1:00:00:00'

test('bulk text import: 3 lines parsed and written', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const form = page.locator('.card.p-3.shadow-sm')
  // HostForm has two .form-select-sm (importMode + template); importMode
  // is first in the DOM, and switching it to 'text' hides the template one.
  await form.locator('select.form-select-sm').first().selectOption('text')

  await form.locator('textarea.form-control').fill(
    `${PREFIX}:01 10.99.20.11 host1\n` +
    `${PREFIX}:02 10.99.20.12 host2\n` +
    `${PREFIX}:03 10.99.20.13 host3`,
  )

  // Client-side parsed count.
  await expect(form.locator('strong')).toHaveText('3')

  await form.locator('.input-group:has(.btn-success) input.form-control').fill(`${CONF_DIR}/e2e-bulkimport.conf`)
  await form.locator('.input-group .btn-success').click()

  const rows = page.locator('tbody tr td.font-monospace', { hasText: PREFIX })
  await expect(rows).toHaveCount(3, { timeout: 10000 })
})
