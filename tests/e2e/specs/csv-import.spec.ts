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

// CSV import via HostForm's csv mode: attach an in-memory CSV of 3 hosts,
// Import, and the success alert(...) must report the count (api/hosts.js
// importCSV → alert csvImportSuccess{count}). Captures the alert message
// to assert the count is correct (A6-adjacent — the CSV path returns count
// properly, unlike the bulk-JSON path), then confirms the rows rendered.

import { test, expect } from '@playwright/test'
import { CONF_DIR } from '../lib/api'

const PREFIX = 'aa:b2:00:00:00'

test('CSV import: upload writes rows and shows count alert', async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.dropdown-toggle')).toBeVisible({ timeout: 15000 })

  const form = page.locator('.card.p-3.shadow-sm')
  await form.locator('select.form-select-sm').first().selectOption('csv')

  const csv =
    'mac,ip,hostname\n' +
    `${PREFIX}:01,10.99.30.11,csv1\n` +
    `${PREFIX}:02,10.99.30.12,csv2\n` +
    `${PREFIX}:03,10.99.30.13,csv3\n`
  await form.locator('input[type="file"]').setInputFiles({
    name: 'e2e.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(csv),
  })

  await form.locator('.input-group:has(.btn-success) input.form-control').fill(`${CONF_DIR}/e2e-csv.conf`)

  // importCSV pops alert("...count=N...") on the POST /hosts/csv round-trip.
  let msg = ''
  page.on('dialog', (d) => { msg = d.message(); d.accept() })
  await form.locator('.input-group .btn-success').click()
  await expect.poll(() => msg, { timeout: 10000 }).not.toBe('')

  // Count must be 3 (CSV path returns count correctly).
  expect(msg).toContain('3')

  const rows = page.locator('tbody tr td.font-monospace', { hasText: PREFIX })
  await expect(rows).toHaveCount(3, { timeout: 10000 })
})
